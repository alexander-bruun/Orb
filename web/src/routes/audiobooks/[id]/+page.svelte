<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/stores";
  import { goto } from "$app/navigation";
  import { audiobooks as abApi } from "$lib/api/audiobooks";
  import { admin as adminApi } from "$lib/api/admin";
  import { authStore } from "$lib/stores/auth";
  import type { Audiobook, AudiobookChapter, AudiobookProgress, AudiobookBookmark } from "$lib/types";
  import { getApiBase } from "$lib/api/base";
  import {
    playAudiobook,
    seekAudiobookMs,
    currentAudiobook,
    abPositionMs,
    abPlaybackState,
    toggleABPlayPause,
    jumpToChapter,
  } from "$lib/stores/audiobookPlayer";

  let book: Audiobook | null = null;
  let chapters: AudiobookChapter[] = [];
  let progress: AudiobookProgress | null = null;
  let bookmarks: AudiobookBookmark[] = [];
  let seriesBooks: Audiobook[] = [];
  let loading = true;
  let error = "";

  $: id = $page.params.id ?? "";
  $: isPlaying = $currentAudiobook?.id === id && $abPlaybackState === "playing";
  $: isActive = $currentAudiobook?.id === id;
  $: isAdmin = $authStore.user?.is_admin === true;

  // ── Inline editing ────────────────────────────────────────────────────────
  type EditField = "title" | "author" | "description" | "series" | "series_index" | "published_year";
  let editingField: EditField | null = null;
  let editValue = "";
  let saving = false;
  let saveError = "";

  function startEdit(field: EditField) {
    if (!isAdmin || !book) return;
    editingField = field;
    saveError = "";
    switch (field) {
      case "title":          editValue = book.title; break;
      case "author":         editValue = book.author_name ?? ""; break;
      case "description":    editValue = book.description ?? ""; break;
      case "series":         editValue = book.series ?? ""; break;
      case "series_index":   editValue = book.series_index != null ? String(book.series_index) : ""; break;
      case "published_year": editValue = book.published_year != null ? String(book.published_year) : ""; break;
    }
  }

  function cancelEdit() {
    editingField = null;
    editValue = "";
    saveError = "";
  }

  async function commitEdit() {
    if (!book || !editingField || saving) return;
    saving = true;
    saveError = "";
    try {
      const body: Record<string, unknown> = { title: book.title };
      switch (editingField) {
        case "title":
          if (!editValue.trim()) { saveError = "Title cannot be empty"; saving = false; return; }
          body.title = editValue.trim();
          break;
        case "author":
          body.author_name = editValue.trim() || null;
          break;
        case "description":
          body.description = editValue.trim() || null;
          break;
        case "series":
          body.series = editValue.trim() || null;
          break;
        case "series_index": {
          const v = editValue.trim();
          body.series_index = v ? parseFloat(v) : null;
          break;
        }
        case "published_year": {
          const v = editValue.trim();
          body.published_year = v ? parseInt(v, 10) : null;
          break;
        }
      }
      await adminApi.updateAudiobookMeta(id, body as Parameters<typeof adminApi.updateAudiobookMeta>[1]);
      // Apply locally
      if (editingField === "title")          book = { ...book, title: body.title as string };
      else if (editingField === "author")         book = { ...book, author_name: body.author_name as string | undefined };
      else if (editingField === "description")    book = { ...book, description: body.description as string | undefined };
      else if (editingField === "series")         book = { ...book, series: body.series as string | undefined };
      else if (editingField === "series_index")   book = { ...book, series_index: body.series_index as number | undefined };
      else if (editingField === "published_year") book = { ...book, published_year: body.published_year as number | undefined };
      editingField = null;
    } catch (e) {
      saveError = e instanceof Error ? e.message : "Save failed";
    } finally {
      saving = false;
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Enter" && editingField !== "description") { e.preventDefault(); commitEdit(); }
    else if (e.key === "Escape") cancelEdit();
  }

  function focusOnMount(node: HTMLElement) {
    setTimeout(() => {
      node.focus();
      if (node instanceof HTMLInputElement || node instanceof HTMLTextAreaElement) node.select();
    }, 0);
  }

  // ── Re-ingest ─────────────────────────────────────────────────────────────
  let refreshing = false;
  let refreshMsg = "";
  let scanning = false;
  let scanMsg = "";

  async function handleRefresh() {
    if (refreshing || !book) return;
    refreshing = true;
    refreshMsg = "";
    try {
      await adminApi.refreshAudiobookMeta(id);
      refreshMsg = "Metadata refreshed";
      const res = await abApi.get(id);
      book = res.audiobook;
      chapters = book.chapters ?? [];
    } catch (e) {
      refreshMsg = e instanceof Error ? e.message : "Refresh failed";
    } finally {
      refreshing = false;
      setTimeout(() => { refreshMsg = ""; }, 3000);
    }
  }

  async function handleRescan() {
    if (scanning) return;
    scanning = true;
    scanMsg = "";
    try {
      await abApi.triggerScan();
      scanMsg = "Scan started";
    } catch (e) {
      scanMsg = e instanceof Error ? e.message : "Scan failed";
    } finally {
      scanning = false;
      setTimeout(() => { scanMsg = ""; }, 3000);
    }
  }

  // ── Helpers ───────────────────────────────────────────────────────────────
  function fmtMs(ms: number): string {
    const totalSecs = Math.floor(ms / 1000);
    const h = Math.floor(totalSecs / 3600);
    const m = Math.floor((totalSecs % 3600) / 60);
    const s = totalSecs % 60;
    if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
    return `${m}:${String(s).padStart(2, "0")}`;
  }

  function fmtDuration(ms: number): string {
    const h = Math.floor(ms / 3_600_000);
    const m = Math.floor((ms % 3_600_000) / 60_000);
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
  }

  function progressPct(chapter: AudiobookChapter): number {
    if (!progress || progress.position_ms <= chapter.start_ms) return 0;
    if (progress.position_ms >= chapter.end_ms) return 100;
    const chLen = chapter.end_ms - chapter.start_ms;
    return ((progress.position_ms - chapter.start_ms) / chLen) * 100;
  }

  function handlePlay() {
    if (!book) return;
    if (isActive) { toggleABPlayPause(); }
    else {
      const startMs = progress?.position_ms ?? 0;
      playAudiobook(book, startMs > 0 ? startMs : undefined);
    }
  }

  function handleChapterPlay(ch: AudiobookChapter) {
    if (!book) return;
    if (isActive) jumpToChapter(ch);
    else playAudiobook(book, ch.start_ms);
  }

  onMount(async () => {
    try {
      const [abRes, progRes, bmRes] = await Promise.all([
        abApi.get(id),
        abApi.getProgress(id).catch(() => null),
        abApi.listBookmarks(id).catch(() => ({ bookmarks: [] })),
      ]);
      book = abRes.audiobook;
      chapters = book.chapters ?? [];
      progress = progRes?.progress ?? null;
      bookmarks = bmRes.bookmarks ?? [];
      if (book.series) {
        abApi.listBySeries(book.series)
          .then((r) => { seriesBooks = (r.audiobooks ?? []).filter((b) => b.id !== id); })
          .catch(() => {});
      }
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : "Failed to load audiobook";
    } finally {
      loading = false;
    }
  });
</script>

<svelte:head>
  <title>{book ? `${book.title} – Orb` : "Audiobook – Orb"}</title>
</svelte:head>

{#if loading}
  <div class="loading-wrap"><div class="spinner"></div></div>
{:else if error}
  <div class="error">{error}</div>
{:else if book}
  <div class="detail">
    <!-- ── Hero ───────────────────────────────────────────────── -->
    <div class="hero">
      <div class="cover-col">
        <div class="cover-wrap">
          {#if book.cover_art_key}
            <img src="{getApiBase()}/covers/audiobook/{book.id}" alt={book.title} class="cover" />
          {:else}
            <div class="cover placeholder">
              <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"/>
                <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"/>
              </svg>
            </div>
          {/if}
        </div>
      </div>

      <div class="meta-col">
        <!-- Series (editable) -->
        <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
        <p class="series-label" class:editable={isAdmin}
          on:click={() => isAdmin ? startEdit("series") : book?.series && goto(`/audiobooks/series/${encodeURIComponent(book.series)}`)}>
          {#if editingField === "series"}
            <!-- svelte-ignore a11y-autofocus -->
            <input class="inline-input series-input" bind:value={editValue}
              on:keydown={onKeydown} on:blur={commitEdit} disabled={saving}
              placeholder="Series name" use:focusOnMount />
          {:else if book.series}
            {book.series}{book.series_index != null ? ` · Book ${book.series_index}` : ""}
            {#if isAdmin}<span class="edit-hint">✎</span>{/if}
          {:else if isAdmin}
            <span class="add-field">+ Add series</span>
          {/if}
        </p>

        <!-- Title (editable) -->
        <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
        <h1 class="title" class:editable={isAdmin} on:click={() => startEdit("title")}>
          {#if editingField === "title"}
            <!-- svelte-ignore a11y-autofocus -->
            <input class="inline-input title-input" bind:value={editValue}
              on:keydown={onKeydown} on:blur={commitEdit} disabled={saving} use:focusOnMount />
          {:else}
            {book.title}{#if isAdmin}<span class="edit-hint">✎</span>{/if}
          {/if}
        </h1>

        <!-- Author (editable) -->
        <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
        <p class="author" class:editable={isAdmin} on:click={() => startEdit("author")}>
          {#if editingField === "author"}
            <!-- svelte-ignore a11y-autofocus -->
            <input class="inline-input" bind:value={editValue}
              on:keydown={onKeydown} on:blur={commitEdit} disabled={saving}
              placeholder="Author name" use:focusOnMount />
          {:else if book.author_name}
            {book.author_name}{#if isAdmin}<span class="edit-hint">✎</span>{/if}
          {:else if isAdmin}
            <span class="add-field">+ Add author</span>
          {/if}
        </p>

        {#if book.narrators?.length}
          <p class="narrator">Narrated by {book.narrators.map((n) => n.name).join(", ")}</p>
        {/if}

        <div class="attrs">
          {#if book.duration_ms}
            <span class="attr">{fmtDuration(book.duration_ms)}</span>
          {/if}
          <!-- Published year (editable) -->
          <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
          <span class="attr" class:editable={isAdmin} on:click={() => startEdit("published_year")}>
            {#if editingField === "published_year"}
              <!-- svelte-ignore a11y-autofocus -->
              <input class="inline-input year-input" type="number" bind:value={editValue}
                on:keydown={onKeydown} on:blur={commitEdit} disabled={saving}
                placeholder="Year" use:focusOnMount />
            {:else if book.published_year}
              {book.published_year}{#if isAdmin}<span class="edit-hint">✎</span>{/if}
            {:else if isAdmin}
              <span class="muted-placeholder">+ Year</span>
            {/if}
          </span>
          <!-- Series index (editable) -->
          {#if book.series || isAdmin}
            <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
            <span class="attr" class:editable={isAdmin} on:click={() => startEdit("series_index")}>
              {#if editingField === "series_index"}
                <!-- svelte-ignore a11y-autofocus -->
                <input class="inline-input year-input" type="number" step="0.1" bind:value={editValue}
                  on:keydown={onKeydown} on:blur={commitEdit} disabled={saving}
                  placeholder="Book #" use:focusOnMount />
              {:else if book.series_index != null}
                Book {book.series_index}{#if isAdmin}<span class="edit-hint">✎</span>{/if}
              {:else if isAdmin}
                <span class="muted-placeholder">+ Book #</span>
              {/if}
            </span>
          {/if}
          {#if chapters.length > 0}
            <span class="attr">{chapters.length} chapter{chapters.length === 1 ? "" : "s"}</span>
          {/if}
          <span class="attr format">{book.format.toUpperCase()}</span>
        </div>

        <!-- Progress bar -->
        {#if progress && progress.position_ms > 0 && book.duration_ms > 0}
          <div class="resume-row">
            <div class="progress-track">
              <div class="progress-fill" style="width: {Math.min(100, (progress.position_ms / book.duration_ms) * 100)}%"></div>
            </div>
            <span class="resume-label">
              {#if progress.completed}Completed{:else}{fmtMs(progress.position_ms)} / {fmtMs(book.duration_ms)}{/if}
            </span>
          </div>
        {/if}

        <div class="actions">
          <button class="btn-play" on:click={handlePlay}>
            {#if isPlaying}
              <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>Pause
            {:else if isActive || (progress && progress.position_ms > 0 && !progress.completed)}
              <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M5 3l14 9-14 9V3z"/></svg>Resume
            {:else}
              <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M5 3l14 9-14 9V3z"/></svg>Play
            {/if}
          </button>
          {#if progress && progress.position_ms > 0 && !progress.completed && !isActive}
            <button class="btn-start-over" on:click={() => book && playAudiobook(book, 0)}>Start over</button>
          {/if}

          {#if isAdmin}
            <button class="btn-admin" on:click={handleRefresh} disabled={refreshing} title="Refresh metadata from Open Library">
              {#if refreshing}<span class="spin-sm">⟳</span> Refreshing…
              {:else}
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
                Refresh metadata
              {/if}
            </button>
            <button class="btn-admin" on:click={handleRescan} disabled={scanning} title="Re-scan audiobook files from disk">
              {#if scanning}<span class="spin-sm">⟳</span> Scanning…
              {:else}
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
                Rescan files
              {/if}
            </button>
          {/if}
        </div>

        {#if refreshMsg || scanMsg || saveError}
          <p class="action-msg" class:error-msg={saveError || refreshMsg.includes("fail") || scanMsg.includes("fail")}>
            {saveError || refreshMsg || scanMsg}
          </p>
        {/if}

        <!-- Description (editable) -->
        <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
        <div class="description-wrap" class:editable={isAdmin && editingField !== "description"}
          on:click={() => isAdmin && editingField !== "description" && startEdit("description")}>
          {#if editingField === "description"}
            <!-- svelte-ignore a11y-autofocus -->
            <textarea class="inline-input desc-input" bind:value={editValue} rows="4"
              on:keydown={(e) => e.key === "Escape" && cancelEdit()}
              on:blur={commitEdit} disabled={saving}
              placeholder="Add a description…" use:focusOnMount></textarea>
          {:else if book.description}
            <p class="description">{book.description}{#if isAdmin}<span class="edit-hint">✎</span>{/if}</p>
          {:else if isAdmin}
            <p class="description add-field">+ Add description</p>
          {/if}
        </div>
      </div>
    </div>

    <!-- ── Chapters ────────────────────────────────────────────── -->
    {#if chapters.length > 0}
      <section class="section">
        <h2 class="section-title">Chapters</h2>
        <div class="chapters">
          {#each chapters as ch (ch.id)}
            {@const pct = progressPct(ch)}
            {@const isCurrent = isActive && $currentAudiobook?.id === id &&
              $abPositionMs >= ch.start_ms && $abPositionMs < ch.end_ms}
            <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
            <div class="chapter-row" class:is-current={isCurrent} on:click={() => handleChapterPlay(ch)}>
              <span class="ch-num">{ch.chapter_num}</span>
              <div class="ch-body">
                <span class="ch-title">{ch.title}</span>
                {#if pct > 0}
                  <div class="ch-progress-track"><div class="ch-progress-fill" style="width:{pct}%"></div></div>
                {/if}
              </div>
              <span class="ch-time">{fmtMs(ch.start_ms)}</span>
              <button class="ch-play" aria-label="Play chapter {ch.chapter_num}"
                on:click|stopPropagation={() => handleChapterPlay(ch)}>
                {#if isCurrent && isPlaying}
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>
                {:else}
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M5 3l14 9-14 9V3z"/></svg>
                {/if}
              </button>
            </div>
          {/each}
        </div>
      </section>
    {/if}

    <!-- ── Bookmarks ──────────────────────────────────────────── -->
    {#if bookmarks.length > 0}
      <section class="section">
        <h2 class="section-title">Bookmarks</h2>
        <div class="bookmarks">
          {#each bookmarks as bm (bm.id)}
            <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
            <div class="bm-row" on:click={() => isActive && seekAudiobookMs(bm.position_ms)}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="bm-icon" aria-hidden="true"><path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z"/></svg>
              <span class="bm-time">{fmtMs(bm.position_ms)}</span>
              {#if bm.note}<span class="bm-note">{bm.note}</span>{/if}
            </div>
          {/each}
        </div>
      </section>
    {/if}

    <!-- ── More in series ─────────────────────────────────────── -->
    {#if seriesBooks.length > 0}
      <section class="section">
        <div class="series-header">
          <h2 class="section-title">More in {book.series}</h2>
          <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
          <span class="series-view-all" on:click={() => goto(`/audiobooks/series/${encodeURIComponent(book!.series!)}`)}>View all</span>
        </div>
        <div class="carousel">
          {#each seriesBooks as sb (sb.id)}
            <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
            <div class="carousel-card" on:click={() => goto(`/audiobooks/${sb.id}`)}>
              <div class="carousel-cover-wrap">
                {#if sb.cover_art_key}
                  <img src="{getApiBase()}/covers/audiobook/{sb.id}" alt={sb.title} class="carousel-cover" loading="lazy" />
                {:else}
                  <div class="carousel-cover placeholder">
                    <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"/><path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"/></svg>
                  </div>
                {/if}
                <button class="carousel-play" aria-label="Play {sb.title}" on:click|stopPropagation={() => playAudiobook(sb)}>
                  <svg width="13" height="13" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M4 2.5l10 5.5-10 5.5V2.5z"/></svg>
                </button>
              </div>
              <span class="carousel-title" title={sb.title}>{sb.title}</span>
              {#if sb.series_index != null}<span class="carousel-idx">Book {sb.series_index}</span>{/if}
            </div>
          {/each}
        </div>
      </section>
    {/if}
  </div>
{/if}

<style>
  .loading-wrap { display: flex; justify-content: center; padding: 80px; }
  .spinner { width: 32px; height: 32px; border: 3px solid var(--border); border-top-color: var(--accent); border-radius: 50%; animation: spin 0.7s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .error { color: var(--text-muted); padding: 40px; text-align: center; font-size: 0.9rem; }

  .detail { display: flex; flex-direction: column; gap: 40px; }

  .hero { display: flex; gap: 32px; align-items: flex-start; }
  .cover-col { flex-shrink: 0; }
  .cover-wrap { width: 180px; height: 270px; border-radius: 10px; overflow: hidden; background: var(--bg-elevated); box-shadow: 0 4px 24px rgba(0,0,0,0.3); }
  .cover { width: 100%; height: 100%; object-fit: cover; display: block; }
  .placeholder { width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; color: var(--text-muted); opacity: 0.35; }

  .meta-col { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 8px; padding-top: 4px; }

  /* ── Inline edit ── */
  .editable { cursor: pointer; }
  .editable:hover .edit-hint { opacity: 1; }
  .edit-hint { opacity: 0; font-size: 0.75em; color: var(--accent); margin-left: 6px; transition: opacity 0.15s; pointer-events: none; }
  .add-field { opacity: 0.45; font-style: italic; font-size: 0.85rem; }
  .muted-placeholder { opacity: 0.5; font-style: italic; }

  .inline-input {
    background: color-mix(in srgb, var(--accent) 8%, var(--bg));
    border: 1.5px solid var(--accent);
    border-radius: 4px;
    color: inherit;
    font: inherit;
    padding: 2px 6px;
    outline: none;
    width: 100%;
    box-sizing: border-box;
  }
  .inline-input:disabled { opacity: 0.6; }
  .title-input { font-size: 1.75rem; font-weight: 700; }
  .series-input { font-size: 0.78rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; }
  .year-input { width: 90px; }
  .desc-input { resize: vertical; line-height: 1.6; font-size: 0.875rem; }

  .description-wrap.editable { cursor: pointer; }

  /* ── Messages ── */
  .action-msg { font-size: 0.8rem; color: var(--accent); margin: 0; }
  .action-msg.error-msg { color: #ef4444; }

  /* ── Admin buttons ── */
  .btn-admin {
    display: inline-flex; align-items: center; gap: 6px;
    background: transparent; border: 1px solid var(--border); border-radius: 20px;
    color: var(--text-muted); font-size: 0.78rem; padding: 5px 12px;
    cursor: pointer; transition: color 0.15s, border-color 0.15s;
  }
  .btn-admin:hover:not(:disabled) { color: var(--text); border-color: var(--text-muted); }
  .btn-admin:disabled { opacity: 0.5; cursor: not-allowed; }
  @keyframes spin-anim { to { transform: rotate(360deg); } }
  .spin-sm { display: inline-block; animation: spin-anim 0.8s linear infinite; }

  /* ── Meta fields ── */
  .series-label { font-size: 0.78rem; font-weight: 600; color: var(--accent); text-transform: uppercase; letter-spacing: 0.06em; margin: 0; cursor: pointer; }
  .series-label:not(.editable):hover { text-decoration: underline; }
  .title { font-size: 1.75rem; font-weight: 700; margin: 0; line-height: 1.2; cursor: default; }
  .title.editable { cursor: pointer; }
  .author { font-size: 1rem; color: var(--text-muted); margin: 0; font-weight: 500; cursor: default; }
  .author.editable { cursor: pointer; }
  .narrator { font-size: 0.85rem; color: var(--text-muted); margin: 0; font-style: italic; }

  .attrs { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; }
  .attr { font-size: 0.75rem; color: var(--text-muted); background: var(--bg-elevated); border: 1px solid var(--border); border-radius: 20px; padding: 3px 10px; cursor: default; }
  .attr.editable { cursor: pointer; }
  .attr.editable:hover { border-color: var(--accent); color: var(--text); }
  .attr.format { font-family: monospace; font-size: 0.7rem; text-transform: uppercase; }

  .resume-row { display: flex; align-items: center; gap: 10px; margin-top: 4px; }
  .progress-track { flex: 1; height: 4px; background: var(--bg-elevated); border-radius: 2px; overflow: hidden; max-width: 180px; }
  .progress-fill { height: 100%; background: var(--accent); border-radius: 2px; }
  .resume-label { font-size: 0.75rem; color: var(--text-muted); white-space: nowrap; }

  .actions { display: flex; gap: 10px; align-items: center; margin-top: 8px; flex-wrap: wrap; }

  .btn-play { display: flex; align-items: center; gap: 8px; background: var(--accent); border: none; border-radius: 24px; color: #fff; font-size: 0.9rem; font-weight: 600; padding: 10px 24px; cursor: pointer; transition: filter 0.15s; }
  .btn-play:hover { filter: brightness(1.1); }

  .btn-start-over { background: transparent; border: 1px solid var(--border); border-radius: 24px; color: var(--text-muted); font-size: 0.875rem; padding: 9px 18px; cursor: pointer; transition: color 0.15s, border-color 0.15s; }
  .btn-start-over:hover { color: var(--text); border-color: var(--text-muted); }

  .description { font-size: 0.875rem; color: var(--text-muted); line-height: 1.6; margin: 8px 0 0; max-width: 600px; display: -webkit-box; -webkit-line-clamp: 5; -webkit-box-orient: vertical; overflow: hidden; }

  /* ── Sections ── */
  .section { display: flex; flex-direction: column; gap: 12px; }
  .section-title { font-size: 1.125rem; font-weight: 600; margin: 0; }
  .chapters { display: flex; flex-direction: column; }
  .chapter-row { display: flex; align-items: center; gap: 12px; padding: 10px 8px; border-radius: 6px; cursor: pointer; transition: background 0.15s; }
  .chapter-row:hover { background: var(--bg-hover); }
  .chapter-row.is-current { background: color-mix(in srgb, var(--accent) 10%, transparent); }
  .ch-num { width: 24px; text-align: right; font-size: 0.75rem; color: var(--text-muted); flex-shrink: 0; }
  .is-current .ch-num { color: var(--accent); font-weight: 600; }
  .ch-body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .ch-title { font-size: 0.875rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .is-current .ch-title { color: var(--accent); font-weight: 500; }
  .ch-progress-track { height: 2px; background: var(--bg-elevated); border-radius: 1px; overflow: hidden; width: 100%; }
  .ch-progress-fill { height: 100%; background: var(--accent); border-radius: 1px; }
  .ch-time { font-size: 0.75rem; color: var(--text-muted); flex-shrink: 0; font-variant-numeric: tabular-nums; }
  .ch-play { width: 28px; height: 28px; border-radius: 50%; background: transparent; border: 1px solid var(--border); color: var(--text-muted); display: flex; align-items: center; justify-content: center; cursor: pointer; flex-shrink: 0; opacity: 0; transition: opacity 0.15s, background 0.15s, color 0.15s; }
  .chapter-row:hover .ch-play { opacity: 1; }
  .is-current .ch-play { opacity: 1; color: var(--accent); border-color: var(--accent); }
  .ch-play:hover { background: var(--bg-elevated); color: var(--text); }

  .bookmarks { display: flex; flex-direction: column; gap: 2px; }
  .bm-row { display: flex; align-items: center; gap: 10px; padding: 8px 12px; border-radius: 6px; cursor: pointer; transition: background 0.15s; }
  .bm-row:hover { background: var(--bg-hover); }
  .bm-icon { color: var(--accent); flex-shrink: 0; }
  .bm-time { font-size: 0.8rem; color: var(--text-muted); font-variant-numeric: tabular-nums; flex-shrink: 0; }
  .bm-note { font-size: 0.85rem; color: var(--text); }

  .series-header { display: flex; align-items: baseline; gap: 12px; }
  .series-view-all { font-size: 0.78rem; color: var(--accent); cursor: pointer; }
  .series-view-all:hover { text-decoration: underline; }

  .carousel { display: flex; gap: 14px; overflow-x: auto; padding-bottom: 8px; scrollbar-width: thin; }
  .carousel-card { flex-shrink: 0; width: 120px; display: flex; flex-direction: column; gap: 5px; cursor: pointer; }
  .carousel-cover-wrap { position: relative; width: 120px; height: 180px; border-radius: 7px; overflow: hidden; background: var(--bg-elevated); }
  .carousel-cover { width: 100%; height: 100%; object-fit: cover; display: block; transition: transform 0.2s; }
  .carousel-card:hover .carousel-cover { transform: scale(1.04); }
  .carousel-cover.placeholder { display: flex; align-items: center; justify-content: center; color: var(--text-muted); opacity: 0.35; width: 100%; height: 100%; }
  .carousel-play { position: absolute; bottom: 6px; right: 6px; width: 30px; height: 30px; border-radius: 50%; background: var(--accent); border: none; color: #fff; display: flex; align-items: center; justify-content: center; cursor: pointer; opacity: 0; transform: translateY(3px); transition: opacity 0.2s, transform 0.2s; }
  .carousel-card:hover .carousel-play { opacity: 1; transform: translateY(0); }
  .carousel-title { font-size: 0.8rem; font-weight: 600; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .carousel-idx { font-size: 0.7rem; color: var(--accent); }

  @media (max-width: 640px) {
    .hero { flex-direction: column; align-items: center; gap: 20px; }
    .cover-wrap { width: 140px; height: 210px; }
    .meta-col { align-items: center; text-align: center; width: 100%; }
    .progress-track { max-width: 180px; }
    .description { max-width: 100%; }
    .title { font-size: 1.35rem; }
    .attrs { justify-content: center; }
    .actions { justify-content: center; }
  }
</style>

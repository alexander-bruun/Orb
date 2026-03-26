<script lang="ts">

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
  } from "$lib/stores/player/audiobookPlayer";
  import {
    downloadAudiobook,
    deleteAudiobookDownload,
    isAudiobookDownloaded,
    downloads,
  } from "$lib/stores/offline/downloads";

  let book: Audiobook | null = null;
  let chapters: AudiobookChapter[] = [];
  let progress: AudiobookProgress | null = null;
  let bookmarks: AudiobookBookmark[] = [];
  let seriesBooks: Audiobook[] = [];
  let loading = true;
  let error = "";
  let loadedId = "";
  let isRestoring = false;

  // Download state
  let isDownloading = false;
  let isDownloaded = false;

  export const snapshot = {
    capture: () => ({ book, chapters, progress, bookmarks, seriesBooks, loadedId }),
    restore: (value) => {
      book = value.book;
      chapters = value.chapters;
      progress = value.progress;
      bookmarks = value.bookmarks;
      seriesBooks = value.seriesBooks;
      loadedId = value.loadedId;
      isRestoring = true;
      loading = false;
    }
  };

  $: id = $page.params.id ?? "";
  // Reload whenever the id param changes (covers both initial mount and
  // same-layout navigation between different books).
  $: if (id && id !== loadedId) loadBook(id);
  $: isPlaying = $currentAudiobook?.id === id && $abPlaybackState === "playing";
  $: isActive = $currentAudiobook?.id === id;
  $: isAdmin = $authStore.user?.is_admin === true;
  $: isDownloaded = book && chapters.length > 0 ? isAudiobookDownloaded(book.id, chapters) : false;

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
      // Start with all current values so unedited fields are not cleared.
      const body: Parameters<typeof adminApi.updateAudiobookMeta>[1] = {
        title: book.title,
        author_name: book.author_name ?? null,
        description: book.description ?? null,
        series: book.series ?? null,
        series_index: book.series_index ?? null,
        published_year: book.published_year ?? null,
      };
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
      await adminApi.updateAudiobookMeta(id, body);
      // Apply locally
      if (editingField === "title")          book = { ...book, title: body.title };
      else if (editingField === "author")         book = { ...book, author_name: body.author_name ?? undefined };
      else if (editingField === "description")    book = { ...book, description: body.description ?? undefined };
      else if (editingField === "series")         book = { ...book, series: body.series ?? undefined };
      else if (editingField === "series_index")   book = { ...book, series_index: body.series_index ?? undefined };
      else if (editingField === "published_year") book = { ...book, published_year: body.published_year ?? undefined };
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

  async function pollForChapters(prevCount: number): Promise<boolean> {
    const deadline = Date.now() + 20_000;
    while (Date.now() < deadline) {
      try {
        const res = await abApi.get(id);
        book = res.audiobook;
        chapters = book.chapters ?? [];
        if (chapters.length > 0 && chapters.length !== prevCount) return true;
      } catch (e) {
        // ignore and keep polling briefly
      }
      await new Promise((r) => setTimeout(r, 1500));
    }
    return false;
  }

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
      const prevCount = chapters.length;
      const audiobookId = $page.params.id;
      if (!audiobookId) return;
      await abApi.triggerRescan(audiobookId);
      scanMsg = "Reingest started";
      const updated = await pollForChapters(prevCount);
      if (updated) scanMsg = "Reingest completed";
      else scanMsg = "Reingest completed, but chapters are still empty";
    } catch (e) {
      scanMsg = e instanceof Error ? e.message : "Scan failed";
    } finally {
      scanning = false;
      setTimeout(() => { scanMsg = ""; }, 3000);
    }
  }

  // ── Download ───────────────────────────────────────────────────────────────

  async function handleDownload() {
    if (!book || isDownloading || !chapters.length) return;
    isDownloading = true;
    try {
      const token = $authStore.token || "";
      await downloadAudiobook(book, token);
      isDownloaded = true;
    } catch (e) {
      console.error("Download failed:", e);
    } finally {
      isDownloading = false;
    }
  }

  async function handleDeleteDownload() {
    if (!book || !chapters.length) return;
    if (!confirm("Delete downloaded audiobook?")) return;
    try {
      await deleteAudiobookDownload(book.id, chapters);
      isDownloaded = false;
    } catch (e) {
      console.error("Delete failed:", e);
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

  function progressPct(chapter: AudiobookChapter, posMs: number): number {
    if (posMs <= chapter.start_ms) return 0;
    if (posMs >= chapter.end_ms) return 100;
    const chLen = chapter.end_ms - chapter.start_ms;
    return ((posMs - chapter.start_ms) / chLen) * 100;
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
    if (isActive) {
      // If we're already in this chapter, toggle pause instead of restarting
      const isSameChapter = $currentAudiobook?.id === id &&
        $abPositionMs >= ch.start_ms && $abPositionMs < ch.end_ms;
      if (isSameChapter) {
        toggleABPlayPause();
      } else {
        jumpToChapter(ch);
      }
    } else {
      playAudiobook(book, ch.start_ms);
    }
  }

  function handleChapterKeydown(e: KeyboardEvent, ch: AudiobookChapter) {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      handleChapterPlay(ch);
    }
  }

  function activateBookmark(bm: AudiobookBookmark) {
    if (isActive) {
      seekAudiobookMs(bm.position_ms);
    }
  }

  function handleBookmarkKeydown(e: KeyboardEvent, bm: AudiobookBookmark) {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      activateBookmark(bm);
    }
  }

  async function loadBook(bookId: string) {
    if (isRestoring && loadedId === bookId) {
      loading = false;
      isRestoring = false;
      return;
    }

    loading = true;
    error = "";
    book = null;
    chapters = [];
    seriesBooks = [];
    try {
      const [abRes, progRes, bmRes] = await Promise.all([
        abApi.get(bookId),
        abApi.getProgress(bookId).catch(() => null),
        abApi.listBookmarks(bookId).catch(() => ({ bookmarks: [] })),
      ]);
      if (bookId !== id) return; // navigated away during load
      loadedId = bookId;
      book = abRes.audiobook;
      chapters = book.chapters ?? [];
      progress = progRes?.progress ?? null;
      bookmarks = bmRes.bookmarks ?? [];
      if (book.series) {
        abApi.listBySeries(book.series)
          .then((r) => { seriesBooks = (r.audiobooks ?? []).filter((b) => b.id !== bookId); })
          .catch(() => {});
      }
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : "Failed to load audiobook";
    } finally {
      loading = false;
      isRestoring = false;
    }
  }

  function handleSeriesBookKeydown(e: KeyboardEvent, book: Audiobook) {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      goto(`/audiobooks/${book.id}`);
    }
  }

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
        
        
        <p class="series-label">
          {#if editingField === "series"}
            
            <input class="inline-input series-input" bind:value={editValue}
              on:keydown={onKeydown} on:blur={commitEdit} disabled={saving}
              placeholder="Series name" use:focusOnMount />
          {:else if book.series}
            {@const seriesName = book.series}
            <button
              type="button"
              class="series series-trigger"
              class:editable={isAdmin}
              title={`View series: ${seriesName}`}
              aria-label={`View series ${seriesName}`}
              on:click={() => isAdmin ? startEdit("series") : goto(`/audiobooks/series/${encodeURIComponent(seriesName)}`)}
            >
              {seriesName}{book.series_index != null ? ` · Book ${book.series_index}` : ""}
              {#if isAdmin}<span class="edit-hint">✎</span>{/if}
            </button>
          {:else if isAdmin}
            <button
              type="button"
              class="series series-trigger editable"
              aria-label="Add series"
              on:click={() => startEdit("series")}
            >
              <span class="add-field">+ Add series</span>
            </button>
          {/if}
        </p>

        <!-- Title (editable) -->
        
        
        <h1 class="title">
          {#if editingField === "title"}
            
            <input class="inline-input title-input" bind:value={editValue}
              on:keydown={onKeydown} on:blur={commitEdit} disabled={saving} use:focusOnMount />
          {:else}
            {#if isAdmin}
              <button
                type="button"
                class="title-trigger editable"
                aria-label={`Edit title ${book.title}`}
                on:click={() => startEdit("title")}
              >
                <span class="title-row">
                  <span class="title-text">{book.title}</span>
                  {#if book.edition}
                    <span class="edition-badge">{book.edition}</span>
                  {/if}
                </span>
                <span class="edit-hint">✎</span>
              </button>
            {:else}
              <span class="title-row">
                <span class="title-text">{book.title}</span>
                {#if book.edition}
                  <span class="edition-badge">{book.edition}</span>
                {/if}
              </span>
            {/if}
          {/if}
        </h1>

        <!-- Author (editable) -->
        
        
        <p class="author">
          {#if editingField === "author"}
            
            <input class="inline-input" bind:value={editValue}
              on:keydown={onKeydown} on:blur={commitEdit} disabled={saving}
              placeholder="Author name" use:focusOnMount />
          {:else if isAdmin}
            <button
              type="button"
              class="author-trigger editable"
              aria-label={book.author_name ? `Edit author ${book.author_name}` : "Add author"}
              on:click={() => startEdit("author")}
            >
              {#if book.author_name}
                {book.author_name}<span class="edit-hint">✎</span>
              {:else}
                <span class="add-field">+ Add author</span>
              {/if}
            </button>
          {:else if book.author_name}
            <span class="author-text" title={book.author_name}>{book.author_name}</span>
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
          
          {#if editingField === "published_year"}
            
            <input class="inline-input year-input" type="number" bind:value={editValue}
              on:keydown={onKeydown} on:blur={commitEdit} disabled={saving}
              placeholder="Year" use:focusOnMount />
          {:else if isAdmin}
            <button
              type="button"
              class="attr attr-trigger editable"
              aria-label="Edit published year"
              on:click={() => startEdit("published_year")}
            >
              {#if book.published_year}
                {book.published_year}<span class="edit-hint">✎</span>
              {:else}
                <span class="muted-placeholder">+ Year</span>
              {/if}
            </button>
          {:else if book.published_year}
            <span class="attr">{book.published_year}</span>
          {/if}
          <!-- Series index (editable) -->
          {#if book.series || isAdmin}
            {#if editingField === "series_index"}
              
                <input class="inline-input year-input" type="number" step="0.1" bind:value={editValue}
                  on:keydown={onKeydown} on:blur={commitEdit} disabled={saving}
                  placeholder="Book #" use:focusOnMount />
            {:else if isAdmin}
              <button
                type="button"
                class="attr attr-trigger editable"
                aria-label="Edit book number"
                on:click={() => startEdit("series_index")}
              >
                {#if book.series_index != null}
                  Book {book.series_index}<span class="edit-hint">✎</span>
                {:else}
                  <span class="muted-placeholder">+ Book #</span>
                {/if}
              </button>
            {:else if book.series_index != null}
              <span class="attr">Book {book.series_index}</span>
            {/if}
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

          {#if isDownloaded}
            <button class="btn-downloaded" on:click={handleDeleteDownload} title="Delete downloaded audiobook">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4l2-3h2l2 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2z"/><circle cx="12" cy="11" r="4"/></svg>
              Downloaded
            </button>
          {:else if !isDownloading}
            <button class="btn-download" on:click={handleDownload} title="Download audiobook for offline use">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="8 17 12 21 16 17"></polyline><line x1="12" y1="12" x2="12" y2="21"></line><path d="M20.88 18.09A5 5 0 0 0 18 9h-1.26A8 8 0 1 0 3 16.29"></path></svg>
              Download
            </button>
          {:else}
            <button class="btn-downloading" disabled>
              <svg class="spin-sm" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true"><circle cx="12" cy="12" r="9" stroke-dasharray="44 13"/></svg>
              Downloading…
            </button>
          {/if}

          {#if isAdmin}
            <button class="btn-admin" on:click={handleRefresh} disabled={refreshing} title="Refresh metadata from Open Library">
              {#if refreshing}<svg class="spin-sm" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true"><circle cx="12" cy="12" r="9" stroke-dasharray="44 13"/></svg> Refreshing…
              {:else}
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
                Refresh metadata
              {/if}
            </button>
            <button class="btn-admin" on:click={handleRescan} disabled={scanning} title="Re-scan audiobook files from disk">
              {#if scanning}<svg class="spin-sm" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true"><circle cx="12" cy="12" r="9" stroke-dasharray="44 13"/></svg> Scanning…
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
        
        <div class="description-wrap">
          {#if editingField === "description"}
            
            <textarea class="inline-input desc-input" bind:value={editValue} rows="4"
              on:keydown={(e) => e.key === "Escape" && cancelEdit()}
              on:blur={commitEdit} disabled={saving}
              placeholder="Add a description…" use:focusOnMount></textarea>
          {:else if isAdmin}
            <button
              type="button"
              class="description-trigger"
              on:click={() => startEdit("description")}
              aria-label="Edit description"
            >
              {#if book.description}
                <p class="description">{book.description}<span class="edit-hint">✎</span></p>
              {:else}
                <p class="description add-field">+ Add description</p>
              {/if}
            </button>
          {:else if book.description}
            <p class="description">{book.description}</p>
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
            {@const pct = progressPct(ch, $abPositionMs)}
            {@const isCurrent = isActive && $currentAudiobook?.id === id &&
              $abPositionMs >= ch.start_ms && $abPositionMs < ch.end_ms}
            
            {@const chDownloaded = $downloads.get(ch.id)?.status === 'done'}
            <div
              class="chapter-row"
              class:is-current={isCurrent}
              role="button"
              tabindex="0"
              aria-label={`Play chapter ${ch.chapter_num}: ${ch.title}`}
              on:click={() => handleChapterPlay(ch)}
              on:keydown={(e) => handleChapterKeydown(e, ch)}
            >
              <span class="ch-num">{ch.chapter_num}</span>
              <div class="ch-body">
                <span class="ch-title">{ch.title}</span>
                {#if pct > 0}
                  <div class="ch-progress-track"><div class="ch-progress-fill" style="width:{pct}%"></div></div>
                {/if}
              </div>
              {#if chDownloaded}
                <svg class="ch-offline-icon" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <title>Downloaded</title>
                  <polyline points="8 17 12 21 16 17"/>
                  <line x1="12" y1="12" x2="12" y2="21"/>
                  <path d="M20.88 18.09A5 5 0 0 0 18 9h-1.26A8 8 0 1 0 3 16.29"/>
                </svg>
              {/if}
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
            
            <div
              class="bm-row"
              role="button"
              tabindex="0"
              aria-label={`Jump to bookmark at ${fmtMs(bm.position_ms)}`}
              on:click={() => activateBookmark(bm)}
              on:keydown={(e) => handleBookmarkKeydown(e, bm)}
            >
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
          
          <button
            type="button"
            class="series-view-all"
            on:click={() => goto(`/audiobooks/series/${encodeURIComponent(book!.series!)}`)}
            aria-label={`View all books in ${book!.series}`}
          >View all</button>
        </div>
        <div class="carousel">
          {#each seriesBooks as sb (sb.id)}
            
            <div
              class="carousel-card"
              role="button"
              tabindex="0"
              aria-label={`Open ${sb.title}`}
              on:click={() => goto(`/audiobooks/${sb.id}`)}
              on:keydown={(e) => handleSeriesBookKeydown(e, sb)}
            >
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

  .series-trigger,
  .title-trigger,
  .author-trigger,
  .attr-trigger,
  .description-trigger {
    background: none;
    border: none;
    color: inherit;
    font: inherit;
    padding: 0;
    margin: 0;
    text-align: left;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
  }
  .description-trigger { width: 100%; justify-content: flex-start; }

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
  .spin-sm { display: inline-block; vertical-align: middle; animation: spin-anim 0.8s linear infinite; }

  /* ── Meta fields ── */
  .series-label { font-size: 0.78rem; font-weight: 600; color: var(--accent); text-transform: uppercase; letter-spacing: 0.06em; margin: 0; }
  .series-trigger:not(.editable):hover { text-decoration: underline; }
  .title { font-size: 1.75rem; font-weight: 700; margin: 0; line-height: 1.2; cursor: default; }
  .title-row { display: inline-flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .title-text { min-width: 0; }
  .edition-badge {
    font-size: 0.7rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: color-mix(in srgb, var(--accent) 70%, var(--text));
    background: color-mix(in srgb, var(--accent) 14%, var(--bg-elevated));
    border: 1px solid color-mix(in srgb, var(--accent) 30%, var(--border));
    border-radius: 999px;
    padding: 4px 10px;
    line-height: 1;
    white-space: nowrap;
  }
  .author { font-size: 1rem; color: var(--text-muted); margin: 0; font-weight: 500; }
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

  .btn-download, .btn-downloaded, .btn-downloading { display: flex; align-items: center; gap: 6px; background: transparent; border: 1px solid var(--border); border-radius: 24px; color: var(--text-muted); font-size: 0.875rem; padding: 9px 16px; cursor: pointer; transition: color 0.15s, border-color 0.15s, background 0.15s; }
  .btn-download:hover { color: var(--text); border-color: var(--text-muted); }
  .btn-downloaded { color: var(--text); border-color: var(--text-muted); background: rgba(0, 255, 0, 0.05); }
  .btn-downloaded:hover { background: rgba(0, 255, 0, 0.1); }
  .btn-downloading { color: var(--text-muted); cursor: not-allowed; opacity: 0.6; }

  .description { font-size: 0.875rem; color: var(--text-muted); line-height: 1.6; margin: 8px 0 0; max-width: 600px; display: -webkit-box; -webkit-line-clamp: 5; line-clamp: 5; -webkit-box-orient: vertical; overflow: hidden; }
  .description-trigger {
    width: 100%;
    border: none;
    background: none;
    padding: 0;
    text-align: left;
    font: inherit;
    cursor: pointer;
  }

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
  .ch-offline-icon { flex-shrink: 0; color: var(--accent); opacity: 0.75; }
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

<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { podcasts as podcastApi } from "$lib/api/podcasts";
  import type {
    Podcast,
    PodcastEpisode,
    PodcastEpisodeProgress,
  } from "$lib/types";
  import { getApiBase } from "$lib/api/base";
  import Spinner from "$lib/components/ui/Spinner.svelte";
  import ConfirmModal from "$lib/components/ui/ConfirmModal.svelte";
  import {
    playEpisode,
    currentEpisode,
    podcastPlaybackState,
  } from "$lib/stores/player/podcastPlayer";

  const PAGE_SIZE = 50;

  let podcast: Podcast | null = null;
  let episodes: PodcastEpisode[] = [];
  let progress: Record<string, PodcastEpisodeProgress> = {};
  let total = 0;
  let currentPage = 1;
  let loading = true;
  let pageLoading = false;

  // ── Inline editing ─────────────────────────────────────────────────────────
  type EditField = "title" | "author" | "description" | "rss_url" | "link";
  let editingField: EditField | null = null;
  let editValue = "";
  let saving = false;
  let saveError = "";

  function startEdit(field: EditField) {
    if (!podcast) return;
    editingField = field;
    saveError = "";
    switch (field) {
      case "title":
        editValue = podcast.title;
        break;
      case "author":
        editValue = podcast.author ?? "";
        break;
      case "description":
        editValue = podcast.description ?? "";
        break;
      case "rss_url":
        editValue = podcast.rss_url;
        break;
      case "link":
        editValue = podcast.link ?? "";
        break;
    }
  }

  function cancelEdit() {
    editingField = null;
    editValue = "";
    saveError = "";
  }

  async function commitEdit() {
    if (!podcast || !editingField || saving) return;
    saving = true;
    saveError = "";
    try {
      const body = {
        title: podcast.title,
        description: podcast.description,
        author: podcast.author,
        rss_url: podcast.rss_url,
        link: podcast.link,
      };
      const field = editingField;
      const val = editValue.trim() || null;
      if (field === "title") {
        if (!editValue.trim()) {
          saveError = "Title cannot be empty";
          saving = false;
          return;
        }
        body.title = editValue.trim();
      } else {
        (body as any)[field] = val;
      }
      await podcastApi.update(podcast.id, body);
      podcast = { ...podcast, ...body };
      editingField = null;
    } catch (e) {
      saveError = e instanceof Error ? e.message : "Save failed";
    } finally {
      saving = false;
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Enter" && editingField !== "description") {
      e.preventDefault();
      commitEdit();
    } else if (e.key === "Escape") cancelEdit();
  }

  function focusOnMount(node: HTMLElement) {
    setTimeout(() => {
      node.focus();
      if (node instanceof HTMLInputElement) node.select();
    }, 0);
  }

  // ── Search & sort state ────────────────────────────────────────────────────
  type SortBy = "pub_date" | "title" | "duration_ms";
  type SortDir = "asc" | "desc";

  let searchQuery = "";
  let sortBy: SortBy = "pub_date";
  let sortDir: SortDir = "desc";
  let searchDebounce: ReturnType<typeof setTimeout> | null = null;

  // ── Delete confirmation ────────────────────────────────────────
  let showDeleteConfirm = false;
  let deleteLoading = false;

  const SORT_OPTIONS: { value: SortBy; label: string }[] = [
    { value: "pub_date", label: "Date" },
    { value: "title", label: "Title" },
    { value: "duration_ms", label: "Duration" },
  ];

  function onSearchInput() {
    if (searchDebounce) clearTimeout(searchDebounce);
    searchDebounce = setTimeout(() => fetchEpisodes(1), 300);
  }

  function toggleSort(field: SortBy) {
    if (sortBy === field) {
      sortDir = sortDir === "desc" ? "asc" : "desc";
    } else {
      sortBy = field;
      sortDir = field === "title" ? "asc" : "desc";
    }
    fetchEpisodes(1);
  }

  function clearSearch() {
    searchQuery = "";
    fetchEpisodes(1);
  }

  // ── Data loading ───────────────────────────────────────────────────────────
  async function load() {
    loading = true;
    currentPage = 1;
    try {
      const id = page.params.id ?? "";
      const [pRes, eRes] = await Promise.all([
        podcastApi.get(id),
        podcastApi.listEpisodes(id, PAGE_SIZE, 0, searchQuery, sortBy, sortDir),
      ]);
      podcast = pRes.podcast;
      episodes = eRes.episodes ?? [];
      total = eRes.total ?? 0;
      await fetchProgress(episodes);
    } catch (err) {
      console.error("Failed to load podcast", err);
    } finally {
      loading = false;
    }
  }

  async function fetchEpisodes(p: number) {
    if (!podcast) return;
    pageLoading = true;
    try {
      const offset = (p - 1) * PAGE_SIZE;
      const res = await podcastApi.listEpisodes(
        podcast.id,
        PAGE_SIZE,
        offset,
        searchQuery,
        sortBy,
        sortDir,
      );
      episodes = res.episodes ?? [];
      total = res.total ?? 0;
      currentPage = p;
      await fetchProgress(episodes);
    } catch (err) {
      console.error("Failed to fetch episodes", err);
    } finally {
      pageLoading = false;
    }
  }

  async function goToPage(p: number) {
    if (!podcast || pageLoading) return;
    fetchEpisodes(p);
  }

  async function fetchProgress(eps: PodcastEpisode[]) {
    if (eps.length === 0) return;
    const results = await Promise.allSettled(
      eps.map((ep) => podcastApi.getProgress(ep.id)),
    );
    const map: Record<string, PodcastEpisodeProgress> = { ...progress };
    results.forEach((r, i) => {
      if (r.status === "fulfilled") map[eps[i].id] = r.value.progress;
    });
    progress = map;
  }

  function fmtDate(date: string) {
    return new Date(date).toLocaleDateString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
    });
  }

  function fmtDuration(ms: number) {
    if (!ms) return "";
    const h = Math.floor(ms / 3600000);
    const m = Math.floor((ms % 3600000) / 60000);
    const s = Math.floor((ms % 60000) / 1000);
    if (h > 0) return `${h}h ${m}m`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
  }

  function progressPct(ep: PodcastEpisode): number {
    const p = progress[ep.id];
    if (!p || !ep.duration_ms) return 0;
    return Math.min(100, (p.position_ms / ep.duration_ms) * 100);
  }

  async function handlePlay(ep: PodcastEpisode) {
    if (!podcast) return;
    await playEpisode(ep, podcast);
  }

  async function handleTogglePlayed(ep: PodcastEpisode) {
    const p = progress[ep.id];
    const nowCompleted = !(p?.completed ?? false);
    await podcastApi.updateProgress(ep.id, p?.position_ms ?? 0, nowCompleted);
    progress = {
      ...progress,
      [ep.id]: {
        ...p,
        episode_id: ep.id,
        user_id: "",
        completed: nowCompleted,
        position_ms: p?.position_ms ?? 0,
        updated_at: new Date().toISOString(),
      },
    };
  }

  async function handleDownload(ep: PodcastEpisode) {
    await podcastApi.download(ep.id);
    episodes = episodes.map((e) =>
      e.id === ep.id ? { ...e, file_key: "downloading" } : e,
    );
  }

  function openDeleteConfirm() {
    showDeleteConfirm = true;
  }

  async function confirmDelete() {
    if (!podcast) return;
    deleteLoading = true;
    try {
      await podcastApi.delete(podcast.id);
      goto("/podcasts");
    } catch (err) {
      console.error("Failed to delete podcast", err);
      deleteLoading = false;
    }
  }

  function cancelDelete() {
    showDeleteConfirm = false;
    deleteLoading = false;
  }

  $: totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  $: pageNumbers = buildPageNumbers(currentPage, totalPages);

  function buildPageNumbers(cur: number, total: number): (number | "...")[] {
    if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1);
    const pages: (number | "...")[] = [1];
    if (cur > 3) pages.push("...");
    for (let i = Math.max(2, cur - 1); i <= Math.min(total - 1, cur + 1); i++)
      pages.push(i);
    if (cur < total - 2) pages.push("...");
    pages.push(total);
    return pages;
  }

  onMount(load);
</script>

<svelte:head><title>{podcast?.title ?? "Podcast"} – Orb</title></svelte:head>

<div class="page">
  {#if loading}
    <div class="hero-skeleton">
      <div class="sk-cover"></div>
      <div class="sk-info">
        <div class="sk-line sk-title"></div>
        <div class="sk-line sk-author"></div>
        <div class="sk-line sk-desc"></div>
        <div class="sk-line sk-meta"></div>
      </div>
    </div>
  {:else if podcast}
    <!-- ── Hero ─────────────────────────────────────────────────── -->
    <div class="hero">
      {#if podcast.cover_art_key}
        <div class="hero-bg" aria-hidden="true">
          <img src="{getApiBase()}/covers/podcast/{podcast.id}" alt="" class="hero-bg-img" />
        </div>
      {/if}
      <div class="hero-body">
        <div class="cover-wrap">
          {#if podcast.cover_art_key}
            <img src="{getApiBase()}/covers/podcast/{podcast.id}" alt={podcast.title} class="cover" />
          {:else}
            <div class="cover placeholder">
              <svg width="56" height="56" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1" stroke-linecap="round" stroke-linejoin="round" style="opacity:0.3">
                <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
                <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
                <line x1="12" y1="19" x2="12" y2="23"/>
                <line x1="8" y1="23" x2="16" y2="23"/>
              </svg>
            </div>
          {/if}
        </div>

        <div class="hero-info">
          <!-- Title -->
          <h1 class="pod-title">
            {#if editingField === "title"}
              <input class="inline-input title-input" bind:value={editValue} on:keydown={onKeydown} on:blur={commitEdit} disabled={saving} use:focusOnMount />
            {:else}
              <button type="button" class="editable-trigger title-trigger" on:click={() => startEdit("title")} aria-label="Edit title">
                {podcast.title}<span class="edit-hint">✎</span>
              </button>
            {/if}
          </h1>

          <!-- Author -->
          <p class="author-field">
            {#if editingField === "author"}
              <input class="inline-input" bind:value={editValue} on:keydown={onKeydown} on:blur={commitEdit} disabled={saving} use:focusOnMount />
            {:else}
              <button type="button" class="editable-trigger author-trigger" on:click={() => startEdit("author")} aria-label="Edit author">
                {#if podcast.author}{podcast.author}{:else}<span class="muted-placeholder">+ Author</span>{/if}
                <span class="edit-hint">✎</span>
              </button>
            {/if}
          </p>

          <!-- Description -->
          <div class="desc-field">
            {#if editingField === "description"}
              <textarea class="inline-input area" bind:value={editValue} on:keydown={onKeydown} on:blur={commitEdit} disabled={saving} use:focusOnMount></textarea>
            {:else}
              <button type="button" class="editable-trigger desc-trigger" on:click={() => startEdit("description")} aria-label="Edit description">
                {#if podcast.description}<span class="description-text">{podcast.description}</span>{:else}<span class="muted-placeholder">+ Description</span>{/if}
                <span class="edit-hint">✎</span>
              </button>
            {/if}
          </div>

          <!-- RSS URL + meta row -->
          <div class="hero-meta">
            <div class="rss-field">
              {#if editingField === "rss_url"}
                <input class="inline-input rss-input" bind:value={editValue} on:keydown={onKeydown} on:blur={commitEdit} disabled={saving} use:focusOnMount />
              {:else}
                <button type="button" class="editable-trigger rss-trigger" on:click={() => startEdit("rss_url")} aria-label="Edit RSS URL">
                  <span class="rss-label">RSS</span>
                  <span class="rss-url-text">{podcast.rss_url}</span>
                  <span class="edit-hint">✎</span>
                </button>
              {/if}
            </div>
            <span class="meta-sep">·</span>
            <span class="ep-count">{total} episode{total !== 1 ? "s" : ""}</span>
          </div>

          {#if saveError}<p class="save-error">{saveError}</p>{/if}

          <div class="hero-actions">
            <button class="btn-delete" title="Delete podcast" on:click={openDeleteConfirm}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
              </svg>
              Delete
            </button>
          </div>
        </div>
      </div>
    </div>

    <div class="episodes">
      <div class="ep-controls">
        <h2>Episodes</h2>
        <div class="controls-right">
          <div class="search-wrap">
            <svg
              class="search-icon"
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <circle cx="11" cy="11" r="8" /><line
                x1="21"
                y1="21"
                x2="16.65"
                y2="16.65"
              />
            </svg>
            <input
              class="search-input"
              type="text"
              placeholder="Search episodes…"
              bind:value={searchQuery}
              on:input={onSearchInput}
            />
            {#if searchQuery}
              <button
                class="search-clear"
                on:click={clearSearch}
                aria-label="Clear search"
              >
                <svg
                  width="12"
                  height="12"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.5"
                  stroke-linecap="round"
                >
                  <line x1="18" y1="6" x2="6" y2="18" /><line
                    x1="6"
                    y1="6"
                    x2="18"
                    y2="18"
                  />
                </svg>
              </button>
            {/if}
          </div>
          <div class="sort-btns">
            {#each SORT_OPTIONS as opt}
              <button
                class="sort-btn"
                class:active={sortBy === opt.value}
                on:click={() => toggleSort(opt.value)}
                title="Sort by {opt.label}"
              >
                {opt.label}
                {#if sortBy === opt.value}
                  <svg
                    width="10"
                    height="10"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                  >
                    {#if sortDir === "asc"}
                      <polyline points="18 15 12 9 6 15" />
                    {:else}
                      <polyline points="6 9 12 15 18 9" />
                    {/if}
                  </svg>
                {/if}
              </button>
            {/each}
          </div>
        </div>
      </div>

      {#if pageLoading}
        <div class="page-loading"><Spinner size={24} /></div>
      {/if}

      {#each episodes as ep (ep.id)}
        {@const isPlaying =
          $currentEpisode?.id === ep.id && $podcastPlaybackState === "playing"}
        {@const isLoading =
          $currentEpisode?.id === ep.id && $podcastPlaybackState === "loading"}
        {@const isActive = $currentEpisode?.id === ep.id}
        {@const pct = progressPct(ep)}
        {@const played = progress[ep.id]?.completed ?? false}

        <div class="episode-row" class:active={isActive} class:played>
          <div class="ep-info">
            <span class="ep-title" class:muted={played}>{ep.title}</span>
            <span class="ep-meta">
              {fmtDate(ep.pub_date)}
              {#if ep.duration_ms}
                <span>·</span>
                <span>{fmtDuration(ep.duration_ms)}</span>
              {/if}
              {#if ep.file_key}
                <span>·</span>
                <span class="downloaded-badge">Downloaded</span>
              {/if}
            </span>
            {#if pct > 0 && !played}
              <div class="ep-progress-bar">
                <div class="ep-progress-fill" style="width: {pct}%"></div>
              </div>
            {/if}
          </div>
          <div class="actions">
            <button
              class="play-btn"
              class:playing={isPlaying}
              on:click={() => handlePlay(ep)}
              aria-label={isPlaying ? "Pause" : "Play episode"}
            >
              {#if isLoading}
                <div class="spin-sm"></div>
              {:else if isPlaying}
                <svg
                  width="14"
                  height="14"
                  viewBox="0 0 24 24"
                  fill="currentColor"
                >
                  <rect x="6" y="4" width="4" height="16" rx="1" />
                  <rect x="14" y="4" width="4" height="16" rx="1" />
                </svg>
              {:else}
                <svg
                  width="14"
                  height="14"
                  viewBox="0 0 24 24"
                  fill="currentColor"
                >
                  <polygon points="5,3 19,12 5,21" />
                </svg>
              {/if}
            </button>

            <button
              class="icon-btn"
              title={played ? "Mark unplayed" : "Mark played"}
              on:click={() => handleTogglePlayed(ep)}
              aria-label={played ? "Mark as unplayed" : "Mark as played"}
            >
              {#if played}
                <svg
                  width="15"
                  height="15"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
                  <path d="M9 11l3 3L22 4" /><path
                    d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"
                  />
                </svg>
              {:else}
                <svg
                  width="15"
                  height="15"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
                  <rect x="3" y="3" width="18" height="18" rx="2" />
                </svg>
              {/if}
            </button>

            {#if !ep.file_key}
              <button
                class="icon-btn"
                title="Download episode"
                on:click={() => handleDownload(ep)}
                aria-label="Download episode"
              >
                <svg
                  width="15"
                  height="15"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                >
                  <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                  <polyline points="7 10 12 15 17 10" />
                  <line x1="12" y1="15" x2="12" y2="3" />
                </svg>
              </button>
            {/if}
          </div>
        </div>
      {/each}

      {#if totalPages > 1}
        <div class="pagination">
          <button
            class="page-btn nav"
            disabled={currentPage === 1 || pageLoading}
            on:click={() => goToPage(currentPage - 1)}
            aria-label="Previous page"
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <polyline points="15 18 9 12 15 6" />
            </svg>
          </button>

          {#each pageNumbers as p}
            {#if p === "..."}
              <span class="page-ellipsis">…</span>
            {:else}
              <button
                class="page-btn"
                class:active={p === currentPage}
                disabled={p === currentPage || pageLoading}
                on:click={() => goToPage(p as number)}>{p}</button
              >
            {/if}
          {/each}

          <button
            class="page-btn nav"
            disabled={currentPage === totalPages || pageLoading}
            on:click={() => goToPage(currentPage + 1)}
            aria-label="Next page"
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <polyline points="9 18 15 12 9 6" />
            </svg>
          </button>
        </div>
      {/if}
    </div>
  {/if}
</div>

<ConfirmModal
  bind:open={showDeleteConfirm}
  title="Delete Podcast"
  message={`Are you sure you want to delete "${podcast?.title}" entirely from the system? This will remove all episodes and subscriptions for all users.`}
  confirmText="Delete"
  cancelText="Cancel"
  variant="danger"
  loading={deleteLoading}
  onConfirm={confirmDelete}
  onCancel={cancelDelete}
/>

<style>
  .page {
    padding: 0;
  }

  /* ── Hero skeleton ── */
  @keyframes sk-pulse { 0%,100%{opacity:.5} 50%{opacity:1} }
  .hero-skeleton {
    display: flex;
    gap: 28px;
    align-items: flex-end;
    padding: 40px 24px 32px;
    margin-bottom: 8px;
  }
  .sk-cover {
    width: 200px;
    height: 200px;
    border-radius: 12px;
    background: var(--bg-elevated);
    flex-shrink: 0;
    animation: sk-pulse 1.4s ease-in-out infinite;
  }
  .sk-info { display: flex; flex-direction: column; gap: 10px; flex: 1; }
  .sk-line {
    border-radius: 4px;
    background: var(--bg-elevated);
    animation: sk-pulse 1.4s ease-in-out infinite;
  }
  .sk-title  { height: 28px; width: min(320px, 80%); }
  .sk-author { height: 14px; width: min(160px, 45%); }
  .sk-desc   { height: 12px; width: min(400px, 90%); }
  .sk-meta   { height: 11px; width: min(200px, 55%); }

  /* ── Hero ── */
  .hero {
    position: relative;
    border-radius: 0;
    overflow: hidden;
    margin-bottom: 32px;
  }
  .hero-bg {
    position: absolute;
    inset: 0;
    z-index: 0;
    overflow: hidden;
  }
  .hero-bg-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    filter: blur(40px) saturate(1.4) brightness(0.5);
    transform: scale(1.1);
  }
  .hero-body {
    position: relative;
    z-index: 1;
    display: flex;
    gap: 32px;
    align-items: flex-end;
    padding: 48px 24px 32px;
    background: linear-gradient(
      to bottom,
      transparent 0%,
      color-mix(in srgb, var(--bg) 30%, transparent) 60%,
      color-mix(in srgb, var(--bg) 75%, transparent) 100%
    );
  }

  .cover-wrap {
    flex-shrink: 0;
    width: 200px;
    height: 200px;
    border-radius: 12px;
    overflow: hidden;
    background: var(--bg-elevated);
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.45);
  }
  .cover {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
  .placeholder {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .hero-info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .pod-title {
    margin: 0;
    font-size: clamp(1.5rem, 3vw, 2.2rem);
    font-weight: 800;
    line-height: 1.15;
    color: var(--text);
  }

  /* ── Inline editing ─────────────────────────────────────────── */
  .editable-trigger {
    background: none;
    border: none;
    color: inherit;
    font: inherit;
    cursor: pointer;
    padding: 2px 4px;
    margin: -2px -4px;
    border-radius: 4px;
    text-align: left;
    display: inline-flex;
    align-items: baseline;
    gap: 6px;
    transition: background 0.15s;
  }
  .editable-trigger:hover { background: color-mix(in srgb, var(--bg-elevated) 60%, transparent); }
  .title-trigger { font-size: clamp(1.5rem, 3vw, 2.2rem); font-weight: 800; }
  .author-trigger { color: var(--accent); font-weight: 600; }
  .desc-trigger {
    color: var(--text-muted);
    font-size: 0.875rem;
    line-height: 1.5;
    max-width: 640px;
    display: flex;
    align-items: flex-start;
  }
  .rss-trigger {
    color: var(--text-muted);
    font-size: 0.78rem;
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .edit-hint {
    color: var(--text-muted);
    opacity: 0;
    font-size: 0.75em;
    flex-shrink: 0;
  }
  .editable-trigger:hover .edit-hint { opacity: 0.7; }
  .muted-placeholder { color: var(--text-muted); font-style: italic; font-weight: 400; }

  .inline-input {
    background: color-mix(in srgb, var(--accent) 8%, var(--bg));
    border: 1.5px solid var(--accent);
    color: var(--text);
    padding: 4px 8px;
    border-radius: 4px;
    font-size: inherit;
    font-family: inherit;
    font-weight: inherit;
    width: 100%;
    outline: none;
    box-sizing: border-box;
  }
  .title-input { font-size: clamp(1.5rem, 3vw, 2.2rem); font-weight: 800; }
  .rss-input { font-size: 0.78rem; max-width: 500px; }
  .area { min-height: 80px; resize: vertical; font-size: 0.875rem; max-width: 640px; }

  .author-field { margin: 0; }
  .desc-field { margin: 0; }

  /* ── Hero meta row ── */
  .hero-meta {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    margin-top: 2px;
  }
  .rss-field { margin: 0; }
  .rss-label {
    font-weight: 600;
    color: var(--text-muted);
    font-size: 0.75rem;
    background: color-mix(in srgb, var(--bg-elevated) 70%, transparent);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 1px 5px;
  }
  .rss-url-text {
    max-width: 300px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.78rem;
    color: var(--text-muted);
  }
  .meta-sep { color: var(--text-muted); opacity: 0.5; font-size: 0.8rem; }
  .ep-count { color: var(--text-muted); font-size: 0.8rem; }
  .description-text { white-space: pre-wrap; }
  .save-error { color: #ef4444; font-size: 0.82rem; margin: 0; }

  /* ── Hero actions ── */
  .hero-actions { display: flex; gap: 8px; margin-top: 6px; }
  .btn-delete {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: 1px solid color-mix(in srgb, #ef4444 40%, var(--border));
    border-radius: 20px;
    color: #ef4444;
    font-size: 0.78rem;
    padding: 5px 14px;
    cursor: pointer;
    transition: background 0.15s, border-color 0.15s;
  }
  .btn-delete:hover { background: rgba(239,68,68,0.1); border-color: #ef4444; }

  /* ── Episodes ───────────────────────────────────────────────── */
  .episodes {
    padding: 0 24px 24px;
  }
  .ep-controls {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 8px;
    flex-wrap: wrap;
  }
  .episodes h2 {
    font-size: 1.1rem;
    font-weight: 600;
    margin: 0;
  }
  .controls-right {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .search-wrap {
    position: relative;
    display: flex;
    align-items: center;
  }
  .search-icon {
    position: absolute;
    left: 8px;
    color: var(--text-muted);
    pointer-events: none;
  }
  .search-input {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    color: var(--text);
    padding: 5px 28px 5px 28px;
    border-radius: 20px;
    font-size: 0.82rem;
    width: 180px;
    outline: none;
    transition:
      border-color 0.15s,
      width 0.2s;
  }
  .search-input:focus {
    border-color: var(--accent);
    width: 220px;
  }
  .search-clear {
    position: absolute;
    right: 8px;
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0;
    display: flex;
    align-items: center;
  }
  .search-clear:hover {
    color: var(--text);
  }

  .sort-btns {
    display: flex;
    gap: 2px;
  }
  .sort-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--text-muted);
    padding: 4px 10px;
    font-size: 0.78rem;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 4px;
    transition: all 0.15s;
  }
  .sort-btn:first-child {
    border-radius: 4px 0 0 4px;
  }
  .sort-btn:last-child {
    border-radius: 0 4px 4px 0;
  }
  .sort-btn:not(:first-child) {
    margin-left: -1px;
  }
  .sort-btn:hover:not(.active) {
    color: var(--text);
    background: var(--bg-elevated);
    z-index: 1;
    position: relative;
  }
  .sort-btn.active {
    background: var(--accent);
    border-color: var(--accent);
    color: white;
    z-index: 1;
    position: relative;
  }

  .page-loading {
    display: flex;
    justify-content: center;
    padding: 16px;
  }

  .episode-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 12px;
    border-bottom: 1px solid var(--border);
    border-radius: 6px;
    gap: 12px;
    transition: background 0.1s;
  }
  .episode-row:hover {
    background: var(--bg-elevated);
  }
  .episode-row.active {
    background: color-mix(in srgb, var(--accent) 8%, transparent);
  }

  .ep-info {
    display: flex;
    flex-direction: column;
    gap: 4px;
    flex: 1;
    min-width: 0;
  }
  .ep-title {
    font-weight: 500;
    font-size: 0.9rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .ep-title.muted {
    color: var(--text-muted);
  }
  .ep-meta {
    font-size: 0.78rem;
    color: var(--text-muted);
    display: flex;
    gap: 6px;
    align-items: center;
  }
  .downloaded-badge {
    color: var(--accent);
    font-size: 0.72rem;
  }

  .ep-progress-bar {
    height: 2px;
    background: var(--border);
    border-radius: 1px;
    margin-top: 2px;
    max-width: 200px;
  }
  .ep-progress-fill {
    height: 100%;
    background: var(--accent);
    border-radius: 1px;
  }

  .actions {
    display: flex;
    gap: 6px;
    align-items: center;
    flex-shrink: 0;
  }

  .play-btn {
    background: var(--accent);
    color: white;
    border: none;
    width: 32px;
    height: 32px;
    border-radius: 50%;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: opacity 0.15s;
  }
  .play-btn:hover {
    opacity: 0.85;
  }

  .icon-btn {
    background: transparent;
    color: var(--text-muted);
    border: none;
    width: 28px;
    height: 28px;
    border-radius: 4px;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: color 0.15s;
  }
  .icon-btn:hover {
    color: var(--text);
  }

  /* ── Pagination ─────────────────────────────────────────────── */
  .pagination {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    padding: 24px 0 8px;
  }

  .page-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--text-muted);
    min-width: 34px;
    height: 34px;
    padding: 0 6px;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.85rem;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.15s;
  }
  .page-btn:hover:not(:disabled) {
    border-color: var(--text-muted);
    color: var(--text);
    background: var(--bg-elevated);
  }
  .page-btn.active {
    background: var(--accent);
    border-color: var(--accent);
    color: white;
    cursor: default;
  }
  .page-btn:disabled:not(.active) {
    opacity: 0.4;
    cursor: not-allowed;
  }
  .page-btn.nav {
    min-width: 34px;
  }
  .page-ellipsis {
    color: var(--text-muted);
    font-size: 0.85rem;
    padding: 0 4px;
    line-height: 34px;
  }

  .spin-sm {
    width: 12px;
    height: 12px;
    border: 2px solid rgba(255, 255, 255, 0.4);
    border-top-color: white;
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  @media (max-width: 640px) {
    .hero-body {
      flex-direction: column;
      align-items: center;
      text-align: center;
      padding: 28px 16px 24px;
      gap: 20px;
    }
    .cover-wrap { width: 160px; height: 160px; }
    .hero-info { align-items: center; width: 100%; }
    .hero-meta { justify-content: center; }
    .hero-actions { justify-content: center; }
    .desc-trigger { max-width: 100%; }
    .rss-url-text { max-width: 180px; }
    .hero-skeleton { flex-direction: column; align-items: center; padding: 24px 16px; }
    .sk-cover { width: 160px; height: 160px; }
    .sk-info { align-items: center; }

    .episodes { padding: 0 16px 20px; }

    /* Episode controls: stack search + sort below heading */
    .ep-controls {
      flex-direction: column;
      align-items: stretch;
      gap: 8px;
    }
    .controls-right {
      flex-direction: column;
      align-items: stretch;
    }
    .search-wrap {
      width: 100%;
    }
    .search-input {
      width: 100%;
      box-sizing: border-box;
    }
    .search-input:focus {
      width: 100%;
    }
    .sort-btns {
      width: 100%;
    }
    .sort-btn {
      flex: 1;
      justify-content: center;
      padding: 6px 8px;
    }

    /* Bigger touch targets on episode rows */
    .episode-row {
      padding: 12px 8px;
    }
    .play-btn {
      width: 38px;
      height: 38px;
    }
    .icon-btn {
      width: 34px;
      height: 34px;
    }
    .btn-delete {
      width: 40px;
      height: 40px;
    }

    /* Keep progress bar full width on mobile */
    .ep-progress-bar {
      max-width: 100%;
    }
  }
</style>

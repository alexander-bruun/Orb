<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { smartPlaylists } from '$lib/api/smartPlaylists';
  import { library } from '$lib/api/library';
  import type { SmartPlaylist, SmartPlaylistRule, SmartPlaylistField, SmartPlaylistOp, Track } from '$lib/types';
  import TrackList from '$lib/components/library/TrackList.svelte';
  import { playTrack } from '$lib/stores/player';

  const FIELDS: { value: SmartPlaylistField; label: string }[] = [
    { value: 'genre',            label: 'Genre' },
    { value: 'year',             label: 'Year' },
    { value: 'artist',           label: 'Artist' },
    { value: 'album',            label: 'Album' },
    { value: 'format',           label: 'Format' },
    { value: 'bit_depth',        label: 'Bit depth' },
    { value: 'duration_ms',      label: 'Duration (ms)' },
    { value: 'play_count',       label: 'Play count' },
    { value: 'rating',           label: 'Rating (1–5)' },
    { value: 'days_since_added', label: 'Days since added' },
    { value: 'days_since_played',label: 'Days since played' },
  ];

  const OPS_TEXT: { value: SmartPlaylistOp; label: string }[] = [
    { value: 'is',          label: 'is' },
    { value: 'is_not',      label: 'is not' },
    { value: 'contains',    label: 'contains' },
    { value: 'not_contains',label: 'does not contain' },
  ];

  const OPS_NUM: { value: SmartPlaylistOp; label: string }[] = [
    { value: 'is',  label: '=' },
    { value: 'gt',  label: '>' },
    { value: 'gte', label: '>=' },
    { value: 'lt',  label: '<' },
    { value: 'lte', label: '<=' },
  ];

  const SORT_FIELDS = [
    { value: 'title',       label: 'Title' },
    { value: 'year',        label: 'Year' },
    { value: 'artist',      label: 'Artist' },
    { value: 'duration_ms', label: 'Duration' },
    { value: 'play_count',  label: 'Play count' },
    { value: 'rating',      label: 'Rating' },
    { value: 'added_at',    label: 'Date added' },
  ];

  const numericFields = new Set<SmartPlaylistField>(['year','bit_depth','duration_ms','play_count','rating','days_since_added','days_since_played']);
  const FORMATS = ['flac','mp3','aac','ogg','opus','wav','alac','aiff','wv','ape'];

  // Suggestion data keyed by field
  let suggestionData: Record<string, string[]> = {};

  async function loadSuggestions() {
    const [genres, artists, albums] = await Promise.allSettled([
      library.genres(),
      library.artists(),
      library.albums(),
    ]);
    suggestionData = {
      genre:  genres.status  === 'fulfilled' ? (genres.value  ?? []).map((g: any) => g.name) : [],
      artist: artists.status === 'fulfilled' ? (artists.value ?? []).map((a: any) => a.name) : [],
      album:  albums.status  === 'fulfilled' ? (albums.value?.items ?? []).map((a: any) => a.title) : [],
      format: FORMATS,
    };
  }

  // Per-rule suggestion state
  let focusedRule: number = -1;
  let suggestionList: string[] = [];
  let highlightedIdx: number = -1;

  function getSuggestions(field: SmartPlaylistField, query: string): string[] {
    if (!query.trim()) return [];
    const list = suggestionData[field] ?? [];
    const q = query.toLowerCase();
    const matches = list.filter(s => s.toLowerCase().includes(q));
    // Exact match first, then starts-with, then contains
    matches.sort((a, b) => {
      const al = a.toLowerCase(), bl = b.toLowerCase();
      const aExact = al === q, bExact = bl === q;
      const aStarts = al.startsWith(q), bStarts = bl.startsWith(q);
      if (aExact !== bExact) return aExact ? -1 : 1;
      if (aStarts !== bStarts) return aStarts ? -1 : 1;
      return a.localeCompare(b);
    });
    return matches.slice(0, 8);
  }

  function onValueInput(i: number) {
    focusedRule = i;
    highlightedIdx = -1;
    suggestionList = getSuggestions(rules[i].field, rules[i].value);
  }

  function onValueFocus(i: number) {
    focusedRule = i;
    highlightedIdx = -1;
    suggestionList = getSuggestions(rules[i].field, rules[i].value);
  }

  function onValueBlur() {
    // Delay so click on suggestion registers first
    setTimeout(() => { focusedRule = -1; suggestionList = []; }, 150);
  }

  function pickSuggestion(i: number, val: string) {
    rules[i].value = val;
    focusedRule = -1;
    suggestionList = [];
  }

  function onValueKeydown(i: number, e: KeyboardEvent) {
    if (!suggestionList.length) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      highlightedIdx = Math.min(highlightedIdx + 1, suggestionList.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      highlightedIdx = Math.max(highlightedIdx - 1, -1);
    } else if (e.key === 'Enter' && highlightedIdx >= 0) {
      e.preventDefault();
      pickSuggestion(i, suggestionList[highlightedIdx]);
    } else if (e.key === 'Escape') {
      focusedRule = -1;
      suggestionList = [];
    }
  }

  function opsFor(field: SmartPlaylistField) {
    return numericFields.has(field) ? OPS_NUM : OPS_TEXT;
  }

  function defaultOp(field: SmartPlaylistField): SmartPlaylistOp {
    return numericFields.has(field) ? 'gt' : 'is';
  }

  let pl: SmartPlaylist | null = null;
  let tracks: Track[] = [];
  let loading = true;
  let tracksLoading = false;
  let saving = false;
  let error = '';

  // Editable copies
  let name = '';
  let description = '';
  let rules: SmartPlaylistRule[] = [];
  let ruleMatch: 'all' | 'any' = 'all';
  let sortBy = 'title';
  let sortDir: 'asc' | 'desc' = 'asc';
  let limitCount: number | '' = '';

  onMount(async () => {
    const id = $page.params.id;
    loadSuggestions(); // fire-and-forget
    try {
      pl = await smartPlaylists.get(id);
      if (pl) {
        name        = pl.name;
        description = pl.description ?? '';
        rules       = pl.rules.map(r => ({ ...r }));
        ruleMatch   = pl.rule_match;
        sortBy      = pl.sort_by;
        sortDir     = pl.sort_dir;
        limitCount  = pl.limit_count ?? '';
      }
      await refreshTracks(id);
    } catch {
      error = 'Failed to load smart playlist.';
    } finally {
      loading = false;
    }
  });

  async function refreshTracks(id: string) {
    tracksLoading = true;
    try {
      tracks = (await smartPlaylists.tracks(id)) ?? [];
    } catch {
      tracks = [];
    } finally {
      tracksLoading = false;
    }
  }

  async function save() {
    if (!pl) return;
    saving = true;
    error = '';
    try {
      pl = await smartPlaylists.update(pl.id, {
        name,
        description,
        rules,
        rule_match: ruleMatch,
        sort_by:    sortBy,
        sort_dir:   sortDir,
        limit_count: limitCount === '' ? null : Number(limitCount),
      });
      await refreshTracks(pl!.id);
    } catch (e: any) {
      error = e?.message ?? 'Save failed';
    } finally {
      saving = false;
    }
  }

  function addRule() {
    rules = [...rules, { field: 'genre', op: 'is', value: '' }];
  }

  function removeRule(i: number) {
    rules = rules.filter((_, idx) => idx !== i);
  }

  function onFieldChange(i: number, field: SmartPlaylistField) {
    rules = rules.map((r, idx) =>
      idx === i ? { ...r, field, op: defaultOp(field) } : r
    );
  }

  function playAll() {
    if (tracks.length > 0) playTrack(tracks[0], tracks);
  }
</script>

<svelte:head><title>{pl?.name ?? 'Smart Playlist'} – Orb</title></svelte:head>

{#if loading}
  <p class="muted">Loading…</p>
{:else if error && !pl}
  <p class="error">{error}</p>
{:else if pl}
  <div class="page">

    <!-- Header -->
    <div class="header">
      <div class="header-left">
        <a href="/playlists" class="back">← Playlists</a>
        <input class="name-input" bind:value={name} placeholder="Playlist name" />
        <input class="desc-input" bind:value={description} placeholder="Description (optional)" />
      </div>
      <div class="header-actions">
        <button class="btn-secondary" on:click={playAll} disabled={tracks.length === 0}>
          ▶ Play all
        </button>
        <button class="btn-primary" on:click={save} disabled={saving}>
          {saving ? 'Saving…' : 'Save & Refresh'}
        </button>
      </div>
    </div>

    {#if error}<p class="error">{error}</p>{/if}

    <!-- Rules section -->
    <section class="section">
      <div class="section-header">
        <h2 class="section-title">Rules</h2>
        <label class="match-label">
          Match
          <select class="select-sm" bind:value={ruleMatch}>
            <option value="all">all</option>
            <option value="any">any</option>
          </select>
          of the following
        </label>
      </div>

      <div class="rules-list">
        {#each rules as rule, i (i)}
          <div class="rule-row">
            <!-- Field -->
            <select class="select-sm" value={rule.field}
              on:change={e => onFieldChange(i, e.currentTarget.value as SmartPlaylistField)}>
              {#each FIELDS as f}
                <option value={f.value}>{f.label}</option>
              {/each}
            </select>
            <!-- Op -->
            <select class="select-sm" bind:value={rules[i].op}>
              {#each opsFor(rule.field) as op}
                <option value={op.value}>{op.label}</option>
              {/each}
            </select>
            <!-- Value -->
            {#if numericFields.has(rule.field)}
              <input class="input-sm" bind:value={rules[i].value} placeholder="value" />
            {:else}
              <div class="suggest-wrap">
                <input
                  class="input-sm"
                  bind:value={rules[i].value}
                  placeholder="value"
                  autocomplete="off"
                  on:input={() => onValueInput(i)}
                  on:focus={() => onValueFocus(i)}
                  on:blur={onValueBlur}
                  on:keydown={e => onValueKeydown(i, e)}
                />
                {#if focusedRule === i && suggestionList.length > 0}
                  <ul class="suggest-list">
                    {#each suggestionList as s, si}
                      <li
                        class="suggest-item"
                        class:highlighted={si === highlightedIdx}
                        on:mousedown|preventDefault={() => pickSuggestion(i, s)}
                      >{s}</li>
                    {/each}
                  </ul>
                {/if}
              </div>
            {/if}
            <button class="btn-remove" title="Remove rule" on:click={() => removeRule(i)}>✕</button>
          </div>
        {/each}
        <button class="btn-add-rule" on:click={addRule}>+ Add rule</button>
      </div>
    </section>

    <!-- Sort & limit -->
    <section class="section options-row">
      <label class="opt-label">
        Sort by
        <select class="select-sm" bind:value={sortBy}>
          {#each SORT_FIELDS as sf}
            <option value={sf.value}>{sf.label}</option>
          {/each}
        </select>
      </label>
      <label class="opt-label">
        Direction
        <select class="select-sm" bind:value={sortDir}>
          <option value="asc">Ascending</option>
          <option value="desc">Descending</option>
        </select>
      </label>
      <label class="opt-label">
        Limit
        <input class="input-sm narrow" type="number" min="1" max="5000"
          bind:value={limitCount} placeholder="unlimited" />
      </label>
    </section>

    <!-- Tracks -->
    <section class="section">
      <div class="section-header">
        <h2 class="section-title">
          Tracks
          {#if !tracksLoading}
            <span class="count">({tracks.length})</span>
          {/if}
        </h2>
        {#if pl.last_built_at}
          <span class="muted">Last built {new Date(pl.last_built_at).toLocaleString()}</span>
        {/if}
      </div>
      {#if tracksLoading}
        <p class="muted">Loading tracks…</p>
      {:else if tracks.length === 0}
        <p class="muted">No tracks match the current rules. Add rules and save to refresh.</p>
      {:else}
        <TrackList {tracks} showCover showDiscNumbers={false} />
      {/if}
    </section>
  </div>
{/if}

<style>
  .page { max-width: 860px; }
  .muted { color: var(--text-muted); font-size: 0.875rem; }
  .error { color: var(--error, #f87171); font-size: 0.875rem; }

  .header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 28px;
    flex-wrap: wrap;
  }
  .header-left { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  .back { color: var(--text-muted); font-size: 0.8rem; text-decoration: none; }
  .back:hover { color: var(--text); }
  .name-input {
    background: transparent;
    border: none;
    border-bottom: 2px solid var(--border);
    color: var(--text);
    font-size: 1.25rem;
    font-weight: 600;
    padding: 2px 0;
    width: 100%;
    max-width: 400px;
  }
  .name-input:focus { outline: none; border-bottom-color: var(--accent); }
  .desc-input {
    background: transparent;
    border: none;
    color: var(--text-muted);
    font-size: 0.85rem;
    padding: 2px 0;
    width: 100%;
    max-width: 400px;
  }
  .desc-input:focus { outline: none; color: var(--text); }
  .header-actions { display: flex; gap: 8px; align-items: center; flex-shrink: 0; }

  .btn-primary {
    background: var(--accent);
    border: none;
    border-radius: 20px;
    color: #fff;
    cursor: pointer;
    font-size: 0.85rem;
    font-weight: 600;
    padding: 7px 18px;
  }
  .btn-primary:hover { filter: brightness(1.1); }
  .btn-primary:disabled { opacity: 0.5; cursor: default; }
  .btn-secondary {
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 20px;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 0.85rem;
    font-weight: 600;
    padding: 6px 16px;
  }
  .btn-secondary:hover { color: var(--text); border-color: var(--text-muted); }
  .btn-secondary:disabled { opacity: 0.4; cursor: default; }

  .section { margin-bottom: 32px; }
  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
    flex-wrap: wrap;
    gap: 8px;
  }
  .section-title { font-size: 1rem; font-weight: 600; margin: 0; }
  .count { font-size: 0.85rem; font-weight: 400; color: var(--text-muted); }

  .match-label {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.8rem;
    color: var(--text-muted);
  }

  .rules-list { display: flex; flex-direction: column; gap: 8px; }
  .rule-row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }

  .select-sm {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text);
    font-size: 0.82rem;
    padding: 5px 8px;
    cursor: pointer;
  }
  .select-sm:focus { outline: none; border-color: var(--accent); }

  .input-sm {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text);
    font-size: 0.82rem;
    padding: 5px 8px;
    flex: 1;
    min-width: 100px;
  }
  .input-sm:focus { outline: none; border-color: var(--accent); }
  .input-sm.narrow { flex: 0 0 100px; }

  .btn-remove {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 0.8rem;
    padding: 4px 6px;
    border-radius: 4px;
  }
  .btn-remove:hover { color: var(--error, #f87171); background: var(--bg-hover); }

  .btn-add-rule {
    align-self: flex-start;
    background: none;
    border: 1px dashed var(--border);
    border-radius: 6px;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 0.8rem;
    padding: 5px 14px;
    margin-top: 4px;
  }
  .btn-add-rule:hover { color: var(--text); border-color: var(--text-muted); }

  .suggest-wrap { position: relative; flex: 1; min-width: 100px; }
  .suggest-wrap .input-sm { width: 100%; box-sizing: border-box; }
  .suggest-list {
    position: absolute;
    top: calc(100% + 3px);
    left: 0;
    right: 0;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    box-shadow: 0 4px 16px rgba(0,0,0,0.25);
    list-style: none;
    margin: 0;
    padding: 4px 0;
    z-index: 100;
    max-height: 220px;
    overflow-y: auto;
  }
  .suggest-item {
    padding: 6px 10px;
    font-size: 0.82rem;
    color: var(--text);
    cursor: pointer;
  }
  .suggest-item:hover, .suggest-item.highlighted {
    background: var(--bg-hover, rgba(255,255,255,0.06));
    color: var(--accent);
  }

  .options-row { display: flex; gap: 20px; flex-wrap: wrap; align-items: flex-end; }
  .opt-label {
    display: flex;
    flex-direction: column;
    gap: 5px;
    font-size: 0.78rem;
    color: var(--text-muted);
  }
</style>

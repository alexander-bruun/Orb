<script lang="ts">
  import {
    eqProfiles,
    activeEQProfile,
    genreEQMappings,
    editingProfileId,
    editingProfile,
    loadEQProfiles,
    applyEQProfile,
    createEQProfile,
    saveEQProfile,
    removeEQProfile,
    setDefaultEQProfile,
    setGenreEQMapping,
    removeGenreEQMapping
  } from '$lib/stores/settings/eq';
  import { exportProfile, importProfileFromFile } from '$lib/api/eq';
  import type { EQBand, EQProfile } from '$lib/types';
  import { DEFAULT_EQ_BANDS } from '$lib/types';
  import { audioEngine } from '$lib/audio/engine';
  import { onMount } from 'svelte';

  // ── State ──────────────────────────────────────────────────
  let loading = true;
  let error = '';
  let saving = false;
  let saveMsg = '';

  // Local editable copy of bands for the current profile
  let localBands: EQBand[] = DEFAULT_EQ_BANDS.map(b => ({ ...b }));
  let localName = '';

  // New-profile creation form
  let creatingNew = false;
  let newProfileName = '';

  // Genre mapping props passed in from parent
  export let genres: { id: string; name: string }[] = [];

  onMount(async () => {
    try {
      await loadEQProfiles();
    } catch (e: any) {
      error = e?.message ?? 'Failed to load EQ profiles.';
    } finally {
      loading = false;
    }
  });

  // Keep local editable bands in sync when the selected profile changes
  $: if ($editingProfile) {
    localBands = $editingProfile.bands.length > 0
      ? $editingProfile.bands.map(b => ({ ...b }))
      : DEFAULT_EQ_BANDS.map(b => ({ ...b }));
    localName = $editingProfile.name;
  }

  // ── Helpers ────────────────────────────────────────────────
  function formatFreq(hz: number): string {
    return hz >= 1000 ? `${hz / 1000}k` : `${hz}`;
  }

  function selectProfile(profile: EQProfile) {
    editingProfileId.set(profile.id);
    applyEQProfile(profile);
  }

  async function handleSave() {
    if (!$editingProfile) return;
    saving = true;
    saveMsg = '';
    error = '';
    try {
      await saveEQProfile($editingProfile.id, localName, localBands);
      saveMsg = 'Profile saved.';
      setTimeout(() => saveMsg = '', 3000);
    } catch (e: any) {
      error = e?.message ?? 'Failed to save.';
    } finally {
      saving = false;
    }
  }

  async function handleDelete() {
    if (!$editingProfile) return;
    if (!confirm(`Delete profile "${$editingProfile.name}"?`)) return;
    error = '';
    try {
      await removeEQProfile($editingProfile.id);
    } catch (e: any) {
      error = e?.message ?? 'Failed to delete.';
    }
  }

  async function handleSetDefault() {
    if (!$editingProfile) return;
    error = '';
    try {
      await setDefaultEQProfile($editingProfile.id);
      applyEQProfile({ ...$editingProfile, is_default: true });
      saveMsg = 'Set as default.';
      setTimeout(() => saveMsg = '', 3000);
    } catch (e: any) {
      error = e?.message ?? 'Failed to set default.';
    }
  }

  async function handleCreate() {
    if (!newProfileName.trim()) return;
    saving = true;
    error = '';
    try {
      const profile = await createEQProfile(newProfileName.trim(), DEFAULT_EQ_BANDS.map(b => ({ ...b })));
      editingProfileId.set(profile.id);
      newProfileName = '';
      creatingNew = false;
    } catch (e: any) {
      error = e?.message ?? 'Failed to create profile.';
    } finally {
      saving = false;
    }
  }

  function handleResetBands() {
    localBands = DEFAULT_EQ_BANDS.map(b => ({ ...b, gain: 0 }));
  }

  function handleExport() {
    if (!$editingProfile) return;
    exportProfile({ ...$editingProfile, bands: localBands });
  }

  let importInput: HTMLInputElement;

  async function handleImport(e: Event) {
    const file = (e.target as HTMLInputElement).files?.[0];
    if (!file) return;
    error = '';
    try {
      const imported = await importProfileFromFile(file);
      if ($editingProfile) {
        localName = imported.name;
        localBands = imported.bands;
        saveMsg = 'Profile imported — click Save to apply.';
      } else {
        // No profile selected — create a new one from the import.
        const profile = await createEQProfile(imported.name, imported.bands);
        editingProfileId.set(profile.id);
        saveMsg = 'Profile imported and created.';
      }
      setTimeout(() => saveMsg = '', 4000);
    } catch (e: any) {
      error = e?.message ?? 'Failed to import profile.';
    } finally {
      if (importInput) importInput.value = '';
    }
  }

  // Genre search filter
  let genreSearch = '';

  // Genre mapping
  async function handleGenreMapping(genreId: string, profileId: string) {
    error = '';
    try {
      if (profileId === '') {
        await removeGenreEQMapping(genreId);
      } else {
        await setGenreEQMapping(genreId, profileId);
      }
    } catch (e: any) {
      error = e?.message ?? 'Failed to save genre mapping.';
    }
  }

  function getMappingForGenre(genreId: string): string {
    return $genreEQMappings.find(m => m.genre_id === genreId)?.profile_id ?? '';
  }
</script>

<div class="eq-editor">
  {#if loading}
    <p class="msg" style="color:var(--text-2)">Loading…</p>
  {:else}
    <!-- ── Profile selector ────────────────────────────────── -->
    <div class="eq-profiles-bar">
      <div class="eq-profile-list">
        {#each $eqProfiles as profile (profile.id)}
          <button
            class="eq-profile-chip"
            class:active={$editingProfileId === profile.id}
            on:click={() => selectProfile(profile)}
          >
            {profile.name}{profile.is_default ? ' ★' : ''}
          </button>
        {/each}
      </div>
      <div class="eq-bar-actions">
        <button class="btn-sm" on:click={() => { creatingNew = !creatingNew; }}>+ New</button>
        <button class="btn-sm" on:click={() => importInput.click()}>Import</button>
        <input bind:this={importInput} type="file" accept=".json" style="display:none" on:change={handleImport} />
      </div>
    </div>

    <!-- ── New-profile form ────────────────────────────────── -->
    {#if creatingNew}
      <div class="eq-new-form">
        <input
          class="form-input eq-new-input"
          type="text"
          placeholder="Profile name"
          bind:value={newProfileName}
          on:keydown={(e) => e.key === 'Enter' && handleCreate()}
        />
        <button class="btn-primary" on:click={handleCreate} disabled={saving || !newProfileName.trim()}>
          Create
        </button>
        <button class="btn-secondary" on:click={() => { creatingNew = false; newProfileName = ''; }}>
          Cancel
        </button>
      </div>
    {/if}

    <!-- ── Band sliders ──────────────────────────────────────── -->
    {#if $editingProfile}
      <div class="eq-name-row">
        <input class="form-input eq-name-input" type="text" bind:value={localName} placeholder="Profile name" />
      </div>

      <div class="eq-bands">
        {#each localBands as band, i}
          <div class="eq-band">
            <span class="eq-band-gain">{band.gain > 0 ? '+' : ''}{band.gain.toFixed(1)}</span>
            <input
              class="eq-slider"
              type="range"
              min="-12"
              max="12"
              step="0.5"
              style="writing-mode: vertical-lr; direction: rtl;"
              bind:value={band.gain}
              on:input={() => {
                // Live-update the audio engine as the slider moves
                audioEngine.setEQ(localBands);
              }}
            />
            <span class="eq-band-freq">{formatFreq(band.frequency)}</span>
          </div>
        {/each}
      </div>

      <div class="eq-actions">
        <button class="btn-primary" on:click={handleSave} disabled={saving}>
          {saving ? 'Saving…' : 'Save'}
        </button>
        <button class="btn-secondary" on:click={handleSetDefault} disabled={saving}>
          Set as default
        </button>
        <button class="btn-secondary" on:click={handleExport}>Export</button>
        <button class="btn-secondary" on:click={handleResetBands}>Reset to flat</button>
        <button class="btn-danger" on:click={handleDelete} disabled={saving}>Delete</button>
      </div>

      {#if saveMsg}<p class="msg msg--ok">{saveMsg}</p>{/if}
      {#if error}<p class="msg msg--error">{error}</p>{/if}

      <!-- ── Genre mappings ─────────────────────────────────── -->
      {#if genres.length > 0}
        <div class="eq-genre-section">
          <h3 class="eq-sub-title">Per-genre EQ</h3>
          <p class="eq-genre-hint">Automatically switch to a profile when playing a genre.</p>

          <!-- Active overrides chips -->
          {#if $genreEQMappings.length > 0}
            <div class="eq-genre-overrides">
              <span class="eq-genre-overrides-label">Active overrides</span>
              <div class="eq-genre-overrides-list">
                {#each $genreEQMappings as mapping}
                  {@const genre = genres.find(g => g.id === mapping.genre_id)}
                  {@const profile = $eqProfiles.find(p => p.id === mapping.profile_id)}
                  {#if genre && profile}
                    <div class="eq-genre-chip">
                      <span class="eq-genre-chip-genre">{genre.name}</span>
                      <span class="eq-genre-chip-arrow">→</span>
                      <span class="eq-genre-chip-profile">{profile.name}</span>
                      <button
                        class="eq-genre-chip-remove"
                        title="Remove override"
                        on:click={() => handleGenreMapping(genre.id, '')}
                      >×</button>
                    </div>
                  {/if}
                {/each}
              </div>
            </div>
          {/if}

          <!-- Search input -->
          <div class="eq-genre-search-row">
            <input
              class="eq-genre-search-input"
              type="search"
              placeholder="Search genres to set override…"
              bind:value={genreSearch}
            />
          </div>

          <!-- Filtered results -->
          {#if genreSearch.trim()}
            {@const filtered = genres.filter(g => g.name.toLowerCase().includes(genreSearch.toLowerCase().trim()))}
            {#if filtered.length > 0}
              <div class="eq-genre-list">
                {#each filtered as genre}
                  <div class="eq-genre-row">
                    <span class="eq-genre-name">{genre.name}</span>
                    <select
                      class="eq-genre-select"
                      value={getMappingForGenre(genre.id)}
                      on:change={(e) => handleGenreMapping(genre.id, (e.target as HTMLSelectElement).value)}
                    >
                      <option value="">— No override —</option>
                      {#each $eqProfiles as p}
                        <option value={p.id}>{p.name}</option>
                      {/each}
                    </select>
                  </div>
                {/each}
              </div>
            {:else}
              <p class="eq-genre-noresult">No genres match "{genreSearch}"</p>
            {/if}
          {/if}
        </div>
      {/if}

    {:else}
      <div class="eq-empty">
        <p>Create a profile to get started with the equalizer.</p>
        <button class="btn-primary" on:click={() => { creatingNew = true; }}>Create first profile</button>
      </div>
      {#if error}<p class="msg msg--error">{error}</p>{/if}
    {/if}
  {/if}
</div>

<style>
  .eq-editor {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  /* Profile chip bar */
  .eq-profiles-bar {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
  }
  .eq-profile-list {
    display: flex;
    flex-wrap: wrap;
    gap: 0.375rem;
    flex: 1;
  }
  .eq-profile-chip {
    padding: 0.25rem 0.75rem;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: var(--surface-2);
    color: var(--text-1);
    cursor: pointer;
    font-size: 0.8125rem;
    transition: background 0.15s, border-color 0.15s;
  }
  .eq-profile-chip.active {
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
  }
  .eq-bar-actions {
    display: flex;
    gap: 0.375rem;
  }
  .btn-sm {
    padding: 0.25rem 0.625rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--surface-2);
    color: var(--text-1);
    cursor: pointer;
    font-size: 0.8125rem;
  }
  .btn-sm:hover { background: var(--surface-3); }

  /* New-profile form */
  .eq-new-form {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }
  .eq-new-input { flex: 1; }

  /* Profile name row */
  .eq-name-row { display: flex; }
  .eq-name-input { max-width: 20rem; }

  /* Band sliders */
  .eq-bands {
    display: flex;
    gap: 0.5rem;
    align-items: flex-end;
    padding: 0.5rem 0;
    overflow-x: auto;
  }
  .eq-band {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.25rem;
    min-width: 2.5rem;
  }
  .eq-band-gain {
    font-size: 0.6875rem;
    color: var(--text-2);
    min-width: 2.5rem;
    text-align: center;
  }
  .eq-slider {
    writing-mode: vertical-lr;
    direction: rtl;
    appearance: slider-vertical;
    -webkit-appearance: slider-vertical;
    width: 1.25rem;
    height: 7rem;
    cursor: pointer;
    accent-color: var(--accent);
  }
  .eq-band-freq {
    font-size: 0.6875rem;
    color: var(--text-2);
  }

  /* Actions row */
  .eq-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  /* Messages */
  .msg { font-size: 0.875rem; margin: 0; }
  .msg--ok  { color: var(--green, #4ade80); }
  .msg--error { color: var(--red, #f87171); }

  /* Genre mappings */
  .eq-genre-section { display: flex; flex-direction: column; gap: 0.625rem; margin-top: 0.5rem; }
  .eq-sub-title { font-size: 0.9375rem; font-weight: 600; margin: 0; }
  .eq-genre-hint { font-size: 0.8125rem; color: var(--text-2); margin: 0; }

  /* Active overrides banner */
  .eq-genre-overrides { display: flex; flex-direction: column; gap: 0.375rem; }
  .eq-genre-overrides-label { font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-2); }
  .eq-genre-overrides-list { display: flex; flex-wrap: wrap; gap: 0.375rem; }
  .eq-genre-chip {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.2rem 0.5rem 0.2rem 0.625rem;
    border-radius: 999px;
    background: color-mix(in srgb, var(--accent) 15%, var(--surface-2));
    border: 1px solid color-mix(in srgb, var(--accent) 35%, transparent);
    font-size: 0.75rem;
  }
  .eq-genre-chip-genre { color: var(--text-1); font-weight: 500; }
  .eq-genre-chip-arrow { color: var(--text-2); }
  .eq-genre-chip-profile { color: var(--accent); }
  .eq-genre-chip-remove {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-2);
    font-size: 0.875rem;
    line-height: 1;
    padding: 0 0 0 0.2rem;
    opacity: 0.7;
  }
  .eq-genre-chip-remove:hover { opacity: 1; color: var(--text-1); }

  /* Search */
  .eq-genre-search-row { display: flex; }
  .eq-genre-search-input {
    width: 100%;
    max-width: 28rem;
    height: 32px;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0 10px;
    color: var(--text-1);
    font-size: 0.8125rem;
    outline: none;
    transition: border-color 0.15s;
  }
  .eq-genre-search-input:focus { border-color: var(--accent); }
  .eq-genre-search-input::placeholder { color: var(--text-2); }

  /* Results list */
  .eq-genre-list {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    max-height: 18rem;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.25rem;
    background: var(--surface-2);
  }
  .eq-genre-row {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    padding: 0.25rem 0.375rem;
    border-radius: 4px;
  }
  .eq-genre-row:hover { background: var(--surface-3); }
  .eq-genre-name { flex: 1; font-size: 0.8125rem; color: var(--text-1); }
  .eq-genre-select {
    flex-shrink: 0;
    width: 13rem;
    height: 28px;
    background: var(--surface-3);
    border: 1px solid var(--border);
    border-radius: 5px;
    padding: 0 6px;
    color: var(--text-1);
    font-size: 0.75rem;
    outline: none;
    cursor: pointer;
    transition: border-color 0.15s;
  }
  .eq-genre-select:focus { border-color: var(--accent); }
  .eq-genre-noresult { font-size: 0.8125rem; color: var(--text-2); margin: 0.25rem 0 0; font-style: italic; }

  /* Empty state */
  .eq-empty { display: flex; flex-direction: column; gap: 0.75rem; padding: 1rem 0; }
  .eq-empty p { color: var(--text-2); margin: 0; }

  /* Text inputs */
  .form-input {
    width: 100%;
    height: 34px;
    background: var(--surface-2);
    border: 1px solid var(--border-2);
    border-radius: 7px;
    padding: 0 10px;
    color: var(--text);
    font-size: 13px;
    font-family: 'DM Mono', monospace;
    outline: none;
    transition: border-color 0.15s;
  }
  .form-input:focus { border-color: var(--accent); }
  .form-input:disabled { opacity: 0.5; }
  .form-input::placeholder { color: var(--text-2); }

  /* Button styles matching app conventions */
  .btn-primary {
    padding: 7px 16px;
    border-radius: 7px;
    border: none;
    background: var(--accent);
    color: #fff;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    transition: opacity 0.15s;
    white-space: nowrap;
  }
  .btn-primary:hover:not(:disabled) { opacity: 0.85; }
  .btn-primary:disabled { opacity: 0.5; cursor: default; }

  :global(.btn-secondary) {
    padding: 0.375rem 0.875rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--surface-2);
    color: var(--text-1);
    cursor: pointer;
    font-size: 0.875rem;
  }
  :global(.btn-secondary:hover) { background: var(--surface-3); }
  :global(.btn-danger) {
    padding: 0.375rem 0.875rem;
    border-radius: 6px;
    border: 1px solid var(--red, #f87171);
    background: transparent;
    color: var(--red, #f87171);
    cursor: pointer;
    font-size: 0.875rem;
  }
  :global(.btn-danger:hover) { background: color-mix(in srgb, var(--red, #f87171) 15%, transparent); }
</style>

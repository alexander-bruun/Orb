<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { goto } from '$app/navigation';
  import { authStore } from '$lib/stores/auth';
  import { admin as adminApi } from '$lib/api/admin';
  import { audiobooks as audiobooksApi } from '$lib/api/audiobooks';
  import { getApiBase } from '$lib/api/base';
  import { isNative } from '$lib/utils/platform';

  /** Returns the effective site base URL to pre-fill the admin setting. */
  function effectiveSiteURL(): string {
    if (isNative()) {
      // On Tauri (desktop/mobile), derive from the configured API base URL.
      return getApiBase().replace(/\/api\/?$/, '');
    }
    // On the web, the frontend origin is the correct base URL.
    return window.location.origin;
  }
  import type {
    AdminSummary, UserPlayStat, TrackPlayCount, ArtistPlayCount,
    DailyPlayCount, StorageStats, InviteToken, AuditLog, Album,
    SiteSettings, IngestProgressEvent, Webhook, WebhookDelivery
  } from '$lib/api/admin';
  import type { Audiobook } from '$lib/types';

  type Tab = 'dashboard' | 'users' | 'library' | 'settings' | 'audit' | 'integrations';
  let activeTab: Tab = 'dashboard';

  // Dashboard
  let summary: AdminSummary | null = null;
  let topTracks: TrackPlayCount[] = [];
  let topArtists: ArtistPlayCount[] = [];
  let playsByDay: DailyPlayCount[] = [];
  let storage: StorageStats | null = null;

  // Users
  let users: UserPlayStat[] = [];
  let invites: InviteToken[] = [];
  let showInviteModal = false;
  let inviteEmail = '';
  let inviteResult: { invite_url: string; expires_at: string } | null = null;
  let inviteLoading = false;
  let inviteError = '';
  let showDeleteConfirm = '';
  let quotaEditUser = '';
  let quotaInputGB = '';

  // Library / jobs
  let ingestRunning = false;
  let ingestProgress: IngestProgressEvent | null = null;
  let ingestLog: string[] = [];
  let ingestES: EventSource | null = null;
  let artworkAlbums: Album[] = [];
  let artworkTotal = 0;
  let artworkOffset = 0;
  let artworkLoading = false;
  let artworkRefetching: Record<string, boolean> = {};
  let showForceScanModal = false;
  let showForceAudiobookModal = false;
  let audiobookScanMsg = '';

  // Audiobook missing metadata
  let noCoverAudiobooks: Audiobook[] = [];
  let noCoverTotal = 0;
  let noCoverOffset = 0;
  let noCoverLoading = false;
  let noSeriesAudiobooks: Audiobook[] = [];
  let noSeriesTotal = 0;
  let noSeriesOffset = 0;
  let noSeriesLoading = false;

  // Settings
  let smtpSettings: SiteSettings = {};
  let smtpSaving = false;
  let smtpTestTo = '';
  let smtpTestResult = '';
  let smtpTestError = '';
  let smtpTestLoading = false;

  // Audit log
  let auditLogs: AuditLog[] = [];
  let auditTotal = 0;
  let auditOffset = 0;

  // Integrations / webhooks
  let webhooks: Webhook[] = [];
  let webhookEvents: string[] = [];
  let showWebhookModal = false;
  let editingWebhook: Webhook | null = null;
  let webhookForm = { url: '', secret: '', description: '', events: [] as string[], enabled: true };
  let webhookSaving = false;
  let webhookError = '';
  let webhookDeliveries: WebhookDelivery[] = [];
  let deliveriesWebhookId = '';
  let showDeliveries = false;
  let webhookTesting: Record<string, boolean> = {};

  // Shared
  let loading = true;
  let error = '';

  onMount(async () => {
    if (!$authStore.user?.is_admin) { goto('/'); return; }
    try {
      [summary, users, topTracks, topArtists, playsByDay] = await Promise.all([
        adminApi.summary(),
        adminApi.users(),
        adminApi.topTracks(10),
        adminApi.topArtists(10),
        adminApi.playsByDay(30)
      ]);
    } catch (e: unknown) {
      error = (e as Error).message ?? 'Failed to load';
    } finally {
      loading = false;
    }
  });

  onDestroy(() => { ingestES?.close(); });

  async function switchTab(tab: Tab) {
    activeTab = tab;
    if (tab === 'users' && invites.length === 0) {
      invites = await adminApi.listInvites().catch(() => []);
    }
    if (tab === 'library' && artworkAlbums.length === 0) {
      await loadArtworkPage();
      await loadNoCoverPage();
      await loadNoSeriesPage();
    }
    if (tab === 'settings' && !smtpSettings.smtp_host) {
      smtpSettings = await adminApi.getSettings().catch(() => ({}));
      smtpTestTo = $authStore.user?.email ?? '';
      // Auto-fill site_base_url from the current origin if not yet configured.
      if (!smtpSettings.site_base_url) {
        smtpSettings = { ...smtpSettings, site_base_url: effectiveSiteURL() };
      }
    }
    if (tab === 'audit' && auditLogs.length === 0) {
      await loadAuditPage();
    }
    if (tab === 'dashboard' && !storage) {
      storage = await adminApi.storageStats().catch(() => null);
    }
    if (tab === 'integrations' && webhooks.length === 0) {
      [webhooks, webhookEvents] = await Promise.all([
        adminApi.listWebhooks().catch(() => []),
        adminApi.listWebhookEvents().catch(() => [])
      ]);
    }
  }

  function maxPlays(data: DailyPlayCount[]) { return Math.max(1, ...data.map(d => d.plays)); }
  function fmtMs(ms: number) {
    const h = Math.floor(ms / 3_600_000);
    const m = Math.floor((ms % 3_600_000) / 60_000);
    return h > 0 ? `${h}h ${m}m` : `${m}m`;
  }
  function fmtBytes(b: number) {
    if (b >= 1e12) return (b / 1e12).toFixed(1) + ' TB';
    if (b >= 1e9)  return (b / 1e9).toFixed(1) + ' GB';
    if (b >= 1e6)  return (b / 1e6).toFixed(1) + ' MB';
    return (b / 1e3).toFixed(0) + ' KB';
  }

  async function toggleAdmin(u: UserPlayStat) {
    try {
      await adminApi.setUserAdmin(u.user_id, !u.is_admin);
      users = users.map(x => x.user_id === u.user_id ? { ...x, is_admin: !x.is_admin } : x);
    } catch (e: unknown) { alert((e as Error).message); }
  }

  async function toggleActive(u: UserPlayStat) {
    try {
      await adminApi.setUserActive(u.user_id, !u.is_active);
      users = users.map(x => x.user_id === u.user_id ? { ...x, is_active: !x.is_active } : x);
    } catch (e: unknown) { alert((e as Error).message); }
  }

  async function confirmDelete(userId: string) {
    try {
      await adminApi.deleteUser(userId);
      users = users.filter(x => x.user_id !== userId);
      showDeleteConfirm = '';
    } catch (e: unknown) { alert((e as Error).message); }
  }

  async function saveQuota(u: UserPlayStat) {
    const bytes = quotaInputGB === '' ? null : Math.round(parseFloat(quotaInputGB) * 1e9);
    try {
      await adminApi.setUserQuota(u.user_id, bytes);
      users = users.map(x => x.user_id === u.user_id ? { ...x, storage_quota_bytes: bytes } : x);
      quotaEditUser = '';
    } catch (e: unknown) { alert((e as Error).message); }
  }

  async function sendInvite() {
    if (!inviteEmail.trim()) return;
    inviteLoading = true; inviteError = ''; inviteResult = null;
    try {
      const res = await adminApi.createInvite(inviteEmail.trim());
      inviteResult = res;
      invites = await adminApi.listInvites().catch(() => invites);
    } catch (e: unknown) {
      inviteError = (e as Error).message ?? 'Failed';
    } finally { inviteLoading = false; }
  }

  async function revokeInvite(token: string) {
    try {
      await adminApi.revokeInvite(token);
      invites = invites.filter(i => i.token !== token);
    } catch (e: unknown) { alert((e as Error).message); }
  }

  async function startScan() {
    if (ingestRunning) return;
    ingestRunning = true; ingestLog = []; ingestProgress = null;
    try {
      await adminApi.triggerScan();
    } catch (e: unknown) {
      ingestLog = [`Error: ${(e as Error).message}`];
      ingestRunning = false;
      return;
    }
    ingestES?.close();
    ingestES = adminApi.openIngestStream((ev) => {
      ingestProgress = ev;
      if (ev.file_path) ingestLog = [...ingestLog.slice(-99), ev.file_path];
      if (ev.type === 'complete' || ev.type === 'error') {
        ingestRunning = false;
        ingestES?.close(); ingestES = null;
        if (summary) adminApi.summary().then(s => { summary = s; }).catch(() => {});
      }
    });
  }

  async function startForceScan() {
    showForceScanModal = false;
    if (ingestRunning) return;
    ingestRunning = true; ingestLog = []; ingestProgress = null;
    try {
      await adminApi.triggerForceScan();
    } catch (e: unknown) {
      ingestLog = [`Error: ${(e as Error).message}`];
      ingestRunning = false;
      return;
    }
    ingestES?.close();
    ingestES = adminApi.openIngestStream((ev) => {
      ingestProgress = ev;
      if (ev.file_path) ingestLog = [...ingestLog.slice(-99), ev.file_path];
      if (ev.type === 'complete' || ev.type === 'error') {
        ingestRunning = false;
        ingestES?.close(); ingestES = null;
        if (summary) adminApi.summary().then(s => { summary = s; }).catch(() => {});
      }
    });
  }

  async function startForceAudiobookScan() {
    showForceAudiobookModal = false;
    if (ingestRunning) return;
    try {
      const res = await audiobooksApi.triggerScan(true);
      audiobookScanMsg = res?.status ?? 'Audiobook re-ingest started';
      setTimeout(() => { if (audiobookScanMsg) audiobookScanMsg = ''; }, 4000);
    } catch (e: unknown) {
      audiobookScanMsg = `Error: ${(e as Error).message}`;
      setTimeout(() => { if (audiobookScanMsg) audiobookScanMsg = ''; }, 6000);
    }
  }

  async function loadArtworkPage() {
    artworkLoading = true;
    try {
      const r = await adminApi.albumsNoCover(50, artworkOffset);
      artworkAlbums = r.albums ?? []; artworkTotal = r.total;
    } catch { artworkAlbums = []; }
    finally { artworkLoading = false; }
  }

  async function refetchCover(albumId: string) {
    artworkRefetching = { ...artworkRefetching, [albumId]: true };
    try {
      await adminApi.refetchAlbumCover(albumId);
      artworkAlbums = artworkAlbums.filter(a => a.id !== albumId);
      artworkTotal = Math.max(0, artworkTotal - 1);
      if (summary) summary = { ...summary, albums_no_cover_art: summary.albums_no_cover_art - 1 };
    } catch (e: unknown) { alert((e as Error).message); }
    finally { artworkRefetching = { ...artworkRefetching, [albumId]: false }; }
  }

  async function refetchAllCovers() {
    for (const a of [...artworkAlbums]) await refetchCover(a.id).catch(() => {});
  }

  async function loadNoCoverPage() {
    noCoverLoading = true;
    try {
      const r = await audiobooksApi.listNoCover(50, noCoverOffset);
      noCoverAudiobooks = r.audiobooks ?? []; noCoverTotal = r.total;
    } catch { noCoverAudiobooks = []; }
    finally { noCoverLoading = false; }
  }

  async function loadNoSeriesPage() {
    noSeriesLoading = true;
    try {
      const r = await audiobooksApi.listNoSeries(50, noSeriesOffset);
      noSeriesAudiobooks = r.audiobooks ?? []; noSeriesTotal = r.total;
    } catch { noSeriesAudiobooks = []; }
    finally { noSeriesLoading = false; }
  }

  async function saveSmtp() {
    smtpSaving = true;
    try { await adminApi.updateSmtpSettings(smtpSettings); }
    catch (e: unknown) { alert((e as Error).message); }
    finally { smtpSaving = false; }
  }

  async function testSmtp() {
    smtpTestLoading = true; smtpTestResult = ''; smtpTestError = '';
    try {
      await adminApi.testSmtp(smtpTestTo);
      smtpTestResult = 'Test email sent!';
    } catch (e: unknown) {
      smtpTestError = (e as Error).message ?? 'Failed';
    } finally { smtpTestLoading = false; }
  }

  async function loadAuditPage(offset = 0) {
    auditOffset = offset;
    const r = await adminApi.auditLogs(50, offset).catch(() => ({ logs: [], total: 0 }));
    auditLogs = r.logs ?? []; auditTotal = r.total;
  }

  function openCreateWebhook() {
    editingWebhook = null;
    webhookForm = { url: '', secret: '', description: '', events: [], enabled: true };
    webhookError = '';
    showWebhookModal = true;
  }

  function openEditWebhook(h: Webhook) {
    editingWebhook = h;
    webhookForm = { url: h.url, secret: h.secret, description: h.description, events: [...h.events], enabled: h.enabled };
    webhookError = '';
    showWebhookModal = true;
  }

  function toggleWebhookEvent(ev: string) {
    if (webhookForm.events.includes(ev)) {
      webhookForm.events = webhookForm.events.filter(e => e !== ev);
    } else {
      webhookForm.events = [...webhookForm.events, ev];
    }
  }

  async function saveWebhook() {
    if (!webhookForm.url.trim()) { webhookError = 'URL is required'; return; }
    webhookSaving = true; webhookError = '';
    try {
      if (editingWebhook) {
        const updated = await adminApi.updateWebhook(editingWebhook.id, {
          url: webhookForm.url, secret: webhookForm.secret,
          description: webhookForm.description, events: webhookForm.events,
          enabled: webhookForm.enabled
        });
        webhooks = webhooks.map(h => h.id === updated.id ? updated : h);
      } else {
        const created = await adminApi.createWebhook({
          url: webhookForm.url, secret: webhookForm.secret,
          description: webhookForm.description, events: webhookForm.events
        });
        webhooks = [created, ...webhooks];
      }
      showWebhookModal = false;
    } catch (e: unknown) {
      webhookError = (e as Error).message ?? 'Failed';
    } finally { webhookSaving = false; }
  }

  async function deleteWebhook(h: Webhook) {
    if (!confirm(`Delete webhook for ${h.url}?`)) return;
    try {
      await adminApi.deleteWebhook(h.id);
      webhooks = webhooks.filter(x => x.id !== h.id);
      if (deliveriesWebhookId === h.id) { showDeliveries = false; }
    } catch (e: unknown) { alert((e as Error).message); }
  }

  async function testWebhook(h: Webhook) {
    webhookTesting = { ...webhookTesting, [h.id]: true };
    try { await adminApi.testWebhook(h.id); }
    catch (e: unknown) { alert((e as Error).message); }
    finally { webhookTesting = { ...webhookTesting, [h.id]: false }; }
  }

  async function viewDeliveries(h: Webhook) {
    deliveriesWebhookId = h.id;
    showDeliveries = true;
    webhookDeliveries = await adminApi.listWebhookDeliveries(h.id).catch(() => []);
  }
</script>

<svelte:head><title>Admin — Orb</title></svelte:head>

<main class="admin-page">
  <div class="admin-header">
    <h1>Admin</h1>
    <div class="tabs-scroll">
      <nav class="tabs">
        {#each (['dashboard','users','library','settings','audit','integrations'] as const) as tab}
          <button class="tab" class:active={activeTab === tab} on:click={() => switchTab(tab)}>
            {tab === 'dashboard' ? 'Dashboard' : tab === 'users' ? 'Users' :
             tab === 'library' ? 'Library & Jobs' : tab === 'settings' ? 'Settings' :
             tab === 'audit' ? 'Audit Log' : 'Integrations'}
          </button>
        {/each}
      </nav>
    </div>
  </div>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else}

  {#if activeTab === 'dashboard'}
    {#if summary}
    <section class="cards">
      <div class="card"><span class="cv">{summary.total_users}</span><span class="cl">Users</span></div>
      <div class="card"><span class="cv">{summary.active_users}</span><span class="cl">Active</span></div>
      <div class="card"><span class="cv">{summary.total_tracks.toLocaleString()}</span><span class="cl">Tracks</span></div>
      <div class="card"><span class="cv">{summary.total_albums.toLocaleString()}</span><span class="cl">Albums</span></div>
      <div class="card"><span class="cv">{summary.total_artists.toLocaleString()}</span><span class="cl">Artists</span></div>
      <div class="card"><span class="cv">{summary.total_plays.toLocaleString()}</span><span class="cl">Plays</span></div>
      <div class="card"><span class="cv">{fmtMs(summary.total_played_ms)}</span><span class="cl">Listened</span></div>
      <div class="card"><span class="cv">{fmtBytes(summary.total_size_bytes)}</span><span class="cl">Storage</span></div>
      <div class="card"><span class="cv">{summary.albums_no_cover_art}</span><span class="cl">No Art</span></div>
    </section>
    {/if}

    <section class="panel">
      <h2>Plays — last 30 days</h2>
      <div class="bar-chart">
        {#each playsByDay as day}
          {@const pct = (day.plays / maxPlays(playsByDay)) * 100}
          <div class="bar-col" title="{day.date}: {day.plays} plays">
            <div class="bar" style="height:{pct}%"></div>
            {#if Number(day.date.slice(-2)) % 5 === 1}
              <span class="bar-date">{day.date.slice(5)}</span>
            {/if}
          </div>
        {/each}
      </div>
    </section>

    <div class="two-col">
      <section class="panel">
        <h2>Top Tracks</h2>
        <div class="table-scroll">
          <table>
            <thead><tr><th>#</th><th>Title</th><th>Artist</th><th>Plays</th></tr></thead>
            <tbody>{#each topTracks as t, i}
              <tr><td class="muted">{i+1}</td><td>{t.title}</td><td class="muted">{t.artist_name ?? '—'}</td><td class="plays">{t.plays}</td></tr>
            {/each}</tbody>
          </table>
        </div>
      </section>
      <section class="panel">
        <h2>Top Artists</h2>
        <div class="table-scroll">
          <table>
            <thead><tr><th>#</th><th>Artist</th><th>Plays</th></tr></thead>
            <tbody>{#each topArtists as a, i}
              <tr><td class="muted">{i+1}</td><td>{a.name}</td><td class="plays">{a.plays}</td></tr>
            {/each}</tbody>
          </table>
        </div>
      </section>
    </div>

    {#if storage}
    <section class="panel">
      <h2>Storage by Format</h2>
      <div class="table-scroll">
        <table>
          <thead><tr><th>Format</th><th>Tracks</th><th>Size</th></tr></thead>
          <tbody>{#each storage.by_format as f}
            <tr><td>{f.format.toUpperCase()}</td><td class="plays">{f.count.toLocaleString()}</td><td class="plays">{fmtBytes(f.size_bytes)}</td></tr>
          {/each}</tbody>
        </table>
      </div>
    </section>
    {/if}

  {:else if activeTab === 'users'}
    <div class="section-header">
      <h2>Users ({users.length})</h2>
      <button class="btn-accent" on:click={() => { showInviteModal = true; inviteEmail = ''; inviteResult = null; inviteError = ''; }}>+ Invite</button>
    </div>
    <section class="panel">
      <div class="table-scroll">
        <table>
          <thead><tr><th>Username</th><th>Email</th><th>Verified</th><th>Plays</th><th>Joined</th><th>Quota</th><th>Active</th><th>Role</th><th></th></tr></thead>
          <tbody>
            {#each users as u}
            <tr class:inactive={!u.is_active}>
              <td>{u.username}</td>
              <td class="muted">{u.email}</td>
              <td>
                {#if u.email_verified}
                  <span class="badge verified-badge" title="Email verified">✓</span>
                {:else}
                  <span class="badge unverified-badge" title="Email not verified">—</span>
                {/if}
              </td>
              <td class="plays">{u.play_count}</td>
              <td class="muted nowrap">{new Date(u.created_at).toLocaleDateString()}</td>
              <td>
                {#if quotaEditUser === u.user_id}
                  <span class="quota-edit">
                    <input type="number" bind:value={quotaInputGB} placeholder="GB" class="quota-input" />
                    <button class="btn-xs" on:click={() => saveQuota(u)}>✓</button>
                    <button class="btn-xs" on:click={() => quotaEditUser = ''}>✕</button>
                  </span>
                {:else}
                  <button class="btn-xs muted" on:click={() => { quotaEditUser = u.user_id; quotaInputGB = u.storage_quota_bytes ? String((u.storage_quota_bytes / 1e9).toFixed(0)) : ''; }}>
                    {u.storage_quota_bytes ? fmtBytes(u.storage_quota_bytes) : '∞'}
                  </button>
                {/if}
              </td>
              <td>
                {#if u.user_id !== $authStore.user?.id}
                  <button class="toggle-btn" class:active={u.is_active} on:click={() => toggleActive(u)}>{u.is_active ? 'Active' : 'Off'}</button>
                {:else}<span class="badge">You</span>{/if}
              </td>
              <td>
                {#if u.user_id !== $authStore.user?.id}
                  <button class="toggle-btn" class:active={u.is_admin} on:click={() => toggleAdmin(u)}>{u.is_admin ? 'Admin' : 'User'}</button>
                {:else}<span class="badge accent">Admin</span>{/if}
              </td>
              <td>
                {#if u.user_id !== $authStore.user?.id}
                  {#if showDeleteConfirm === u.user_id}
                    <span class="delete-confirm">
                      <button class="btn-xs danger" on:click={() => confirmDelete(u.user_id)}>Delete</button>
                      <button class="btn-xs" on:click={() => showDeleteConfirm = ''}>Cancel</button>
                    </span>
                  {:else}
                    <button class="btn-xs muted" on:click={() => showDeleteConfirm = u.user_id}>✕</button>
                  {/if}
                {/if}
              </td>
            </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </section>

    <div class="section-header" style="margin-top:1.5rem">
      <h2>Pending Invites</h2>
    </div>
    {#if invites.length === 0}
      <p class="muted" style="padding:0 0.25rem;font-size:0.85rem">No pending invites.</p>
    {:else}
    <section class="panel">
      <div class="table-scroll">
        <table>
          <thead><tr><th>Email</th><th>Expires</th><th>Status</th><th></th></tr></thead>
          <tbody>{#each invites as inv}
            <tr>
              <td>{inv.email}</td>
              <td class="muted nowrap">{new Date(inv.expires_at).toLocaleDateString()}</td>
              <td><span class="badge" class:accent={!inv.used_at}>{inv.used_at ? 'Used' : 'Pending'}</span></td>
              <td>{#if !inv.used_at}<button class="btn-xs muted" on:click={() => revokeInvite(inv.token)}>Revoke</button>{/if}</td>
            </tr>
          {/each}</tbody>
        </table>
      </div>
    </section>
    {/if}

  {:else if activeTab === 'library'}
    <section class="panel">
      <div class="section-header">
        <h2>Ingest</h2>
        <div class="scan-split-btn">
          <button class="btn-accent scan-main" disabled={ingestRunning} on:click={startScan}>{ingestRunning ? 'Scanning…' : 'Trigger Scan'}</button>
          <button class="btn-accent scan-arrow" disabled={ingestRunning} on:click={() => showForceScanModal = true} title="Force rescan entire library" aria-label="Force rescan options">⭯</button>
        </div>
        <div style="display:flex; gap:0.5rem; margin-left:auto">
          <button class="btn-accent" disabled={ingestRunning} on:click={() => showForceScanModal = true}>Re-ingest Music</button>
          <button class="btn-accent" disabled={ingestRunning} on:click={() => showForceAudiobookModal = true}>Re-ingest Audiobooks</button>
        </div>
      </div>
      {#if ingestProgress}
        <div class="progress-bar-wrap">
          <div class="progress-bar" style="width:{ingestProgress.total > 0 ? Math.round(ingestProgress.done / ingestProgress.total * 100) : 0}%"></div>
        </div>
        <p class="progress-info muted">
          {ingestProgress.done}/{ingestProgress.total} · {ingestProgress.skipped} skipped · {ingestProgress.errors} errors
          {#if ingestProgress.type === 'complete'} · <strong style="color:var(--accent)">Done</strong>{/if}
        </p>
      {/if}
      {#if ingestLog.length > 0}
        <div class="ingest-log">{#each ingestLog as line}<div class="log-line">{line}</div>{/each}</div>
      {/if}
      {#if audiobookScanMsg}
        <p class="progress-info muted" style="margin-top:0.5rem">{audiobookScanMsg}</p>
      {/if}
    </section>

    <section class="panel" style="margin-top:1.25rem">
      <div class="section-header">
        <h2>Albums Without Artwork ({artworkTotal})</h2>
        {#if artworkAlbums.length > 0}
          <button class="btn-accent" on:click={refetchAllCovers} disabled={artworkLoading}>Re-fetch All</button>
        {/if}
      </div>
      {#if artworkLoading}
        <p class="muted">Loading…</p>
      {:else if artworkAlbums.length === 0}
        <p class="muted">All albums have artwork.</p>
      {:else}
        <div class="table-scroll">
          <table>
            <thead><tr><th>Album</th><th>Artist</th><th></th></tr></thead>
            <tbody>{#each artworkAlbums as a}
              <tr>
                <td>{a.title}</td>
                <td class="muted">{a.artist_name ?? '—'}</td>
                <td><button class="btn-xs" disabled={artworkRefetching[a.id]} on:click={() => refetchCover(a.id)}>{artworkRefetching[a.id] ? '…' : 'Re-fetch'}</button></td>
              </tr>
            {/each}</tbody>
          </table>
        </div>
        {#if artworkTotal > 50}
          <div class="pagination">
            <button disabled={artworkOffset === 0} on:click={() => { artworkOffset -= 50; loadArtworkPage(); }}>← Prev</button>
            <span class="muted">{artworkOffset+1}–{Math.min(artworkOffset+50, artworkTotal)} of {artworkTotal}</span>
            <button disabled={artworkOffset+50 >= artworkTotal} on:click={() => { artworkOffset += 50; loadArtworkPage(); }}>Next →</button>
          </div>
        {/if}
      {/if}
    </section>

    <section class="panel" style="margin-top:1.25rem">
      <div class="section-header">
        <h2>Audiobooks Without Cover Art ({noCoverTotal})</h2>
      </div>
      {#if noCoverLoading}
        <p class="muted">Loading…</p>
      {:else if noCoverTotal === 0}
        <p class="muted">All audiobooks have cover art.</p>
      {:else}
        <div class="table-scroll">
          <table>
            <thead><tr><th>Title</th><th>Author</th></tr></thead>
            <tbody>{#each noCoverAudiobooks as ab}
              <tr>
                <td>{ab.title}</td>
                <td class="muted">{ab.author_name ?? '—'}</td>
              </tr>
            {/each}</tbody>
          </table>
        </div>
        {#if noCoverTotal > 50}
          <div class="pagination">
            <button disabled={noCoverOffset === 0} on:click={() => { noCoverOffset -= 50; loadNoCoverPage(); }}>← Prev</button>
            <span class="muted">{noCoverOffset+1}–{Math.min(noCoverOffset+50, noCoverTotal)} of {noCoverTotal}</span>
            <button disabled={noCoverOffset+50 >= noCoverTotal} on:click={() => { noCoverOffset += 50; loadNoCoverPage(); }}>Next →</button>
          </div>
        {/if}
      {/if}
    </section>

    <section class="panel" style="margin-top:1.25rem">
      <div class="section-header">
        <h2>Audiobooks Without Series ({noSeriesTotal})</h2>
      </div>
      {#if noSeriesLoading}
        <p class="muted">Loading…</p>
      {:else if noSeriesTotal === 0}
        <p class="muted">All audiobooks have series information.</p>
      {:else}
        <div class="table-scroll">
          <table>
            <thead><tr><th>Title</th><th>Author</th></tr></thead>
            <tbody>{#each noSeriesAudiobooks as ab}
              <tr>
                <td>{ab.title}</td>
                <td class="muted">{ab.author_name ?? '—'}</td>
              </tr>
            {/each}</tbody>
          </table>
        </div>
        {#if noSeriesTotal > 50}
          <div class="pagination">
            <button disabled={noSeriesOffset === 0} on:click={() => { noSeriesOffset -= 50; loadNoSeriesPage(); }}>← Prev</button>
            <span class="muted">{noSeriesOffset+1}–{Math.min(noSeriesOffset+50, noSeriesTotal)} of {noSeriesTotal}</span>
            <button disabled={noSeriesOffset+50 >= noSeriesTotal} on:click={() => { noSeriesOffset += 50; loadNoSeriesPage(); }}>Next →</button>
          </div>
        {/if}
      {/if}
    </section>

  {:else if activeTab === 'settings'}
    <section class="panel">
      <h2>SMTP / Email</h2>
      <form class="settings-form" on:submit|preventDefault={saveSmtp}>
        <div class="form-row"><label for="smtp-host">Host</label><input id="smtp-host" bind:value={smtpSettings.smtp_host} placeholder="smtp.example.com" /></div>
        <div class="form-row"><label for="smtp-port">Port</label><input id="smtp-port" bind:value={smtpSettings.smtp_port} placeholder="587" type="number" /></div>
        <div class="form-row"><label for="smtp-user">Username</label><input id="smtp-user" bind:value={smtpSettings.smtp_username} /></div>
        <div class="form-row"><label for="smtp-pass">Password</label><input id="smtp-pass" type="password" bind:value={smtpSettings.smtp_password} placeholder="••••••••" /></div>
        <div class="form-row"><label for="smtp-from">From Address</label><input id="smtp-from" bind:value={smtpSettings.smtp_from_address} placeholder="orb@example.com" /></div>
        <div class="form-row"><label for="smtp-name">From Name</label><input id="smtp-name" bind:value={smtpSettings.smtp_from_name} placeholder="Orb Music" /></div>
        <div class="form-row"><label for="smtp-tls">TLS (port 465)</label><input id="smtp-tls" type="checkbox" checked={smtpSettings.smtp_tls === 'true'} on:change={(e) => { smtpSettings = { ...smtpSettings, smtp_tls: (e.target as HTMLInputElement).checked ? 'true' : 'false' }; }} /></div>
        <div class="form-row">
          <label for="smtp-url">Site Base URL</label>
          <div style="display:flex;flex-direction:column;gap:4px;flex:1">
            <input id="smtp-url" bind:value={smtpSettings.site_base_url} placeholder="https://music.example.com" />
            <span style="font-size:0.72rem;color:var(--text-muted)">Used in invite and verification email links. Auto-detected from your browser if left blank.</span>
          </div>
        </div>
        <div style="padding-top:0.25rem">
          <button class="btn-accent" type="submit" disabled={smtpSaving}>{smtpSaving ? 'Saving…' : 'Save Settings'}</button>
        </div>
      </form>
      <div style="margin-top:1.5rem">
        <h3>Test Email</h3>
        <div class="test-row">
          <input bind:value={smtpTestTo} placeholder="recipient@example.com" />
          <button class="btn-accent" on:click={testSmtp} disabled={smtpTestLoading}>{smtpTestLoading ? 'Sending…' : 'Send Test'}</button>
        </div>
        {#if smtpTestResult}<p class="success" style="margin-top:0.5rem">{smtpTestResult}</p>{/if}
        {#if smtpTestError}<p class="error" style="margin-top:0.5rem">{smtpTestError}</p>{/if}
      </div>
    </section>

  {:else if activeTab === 'audit'}
    <section class="panel">
      <div class="section-header">
        <h2>Audit Log ({auditTotal})</h2>
        <button class="btn-xs" on:click={() => loadAuditPage(0)}>Refresh</button>
      </div>
      <div class="table-scroll">
        <table>
          <thead><tr><th>Time</th><th>Actor</th><th>Action</th><th>Target</th><th>Detail</th></tr></thead>
          <tbody>{#each auditLogs as l}
            <tr>
              <td class="muted nowrap">{new Date(l.created_at).toLocaleString()}</td>
              <td>{l.actor_name ?? l.actor_id ?? 'system'}</td>
              <td><code>{l.action}</code></td>
              <td class="muted">{[l.target_type, l.target_id?.slice(0,8)].filter(Boolean).join(' #')}</td>
              <td class="muted detail">{l.detail ? JSON.stringify(l.detail) : ''}</td>
            </tr>
          {/each}</tbody>
        </table>
      </div>
      {#if auditTotal > 50}
        <div class="pagination">
          <button disabled={auditOffset === 0} on:click={() => loadAuditPage(auditOffset-50)}>← Prev</button>
          <span class="muted">{auditOffset+1}–{Math.min(auditOffset+50, auditTotal)} of {auditTotal}</span>
          <button disabled={auditOffset+50 >= auditTotal} on:click={() => loadAuditPage(auditOffset+50)}>Next →</button>
        </div>
      {/if}
    </section>

  {:else if activeTab === 'integrations'}
    <section class="panel">
      <div class="section-header">
        <h2>Webhooks ({webhooks.length})</h2>
        <button class="btn-accent" on:click={openCreateWebhook}>+ Add Webhook</button>
      </div>
      <p class="muted" style="font-size:0.82rem;margin-bottom:1rem">
        Orb will POST a signed JSON payload to each URL when the subscribed events occur.
        Verify deliveries using the <code>X-Orb-Signature</code> header (HMAC-SHA256).
      </p>
      {#if webhooks.length === 0}
        <p class="muted">No webhooks configured.</p>
      {:else}
        <div class="table-scroll">
          <table>
            <thead><tr><th>URL</th><th>Description</th><th>Events</th><th>Status</th><th></th></tr></thead>
            <tbody>
              {#each webhooks as h}
              <tr>
                <td class="wh-url">{h.url}</td>
                <td class="muted">{h.description || '—'}</td>
                <td class="muted" style="font-size:0.75rem">{h.events.length > 0 ? h.events.join(', ') : 'none'}</td>
                <td>
                  <span class="badge" class:accent={h.enabled}>{h.enabled ? 'Active' : 'Disabled'}</span>
                </td>
                <td>
                  <span class="row-actions">
                    <button class="btn-xs" on:click={() => testWebhook(h)} disabled={webhookTesting[h.id]}>
                      {webhookTesting[h.id] ? '…' : 'Test'}
                    </button>
                    <button class="btn-xs" on:click={() => viewDeliveries(h)}>Deliveries</button>
                    <button class="btn-xs" on:click={() => openEditWebhook(h)}>Edit</button>
                    <button class="btn-xs danger" on:click={() => deleteWebhook(h)}>✕</button>
                  </span>
                </td>
              </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>

    {#if showDeliveries}
    <section class="panel">
      <div class="section-header">
        <h2>Recent Deliveries</h2>
        <button class="btn-xs" on:click={() => showDeliveries = false}>Close</button>
      </div>
      {#if webhookDeliveries.length === 0}
        <p class="muted">No deliveries recorded yet.</p>
      {:else}
        <div class="table-scroll">
          <table>
            <thead><tr><th>Time</th><th>Event</th><th>Status</th><th>Error</th></tr></thead>
            <tbody>
              {#each webhookDeliveries as d}
              <tr>
                <td class="muted nowrap">{new Date(d.delivered_at).toLocaleString()}</td>
                <td><code>{d.event}</code></td>
                <td>
                  {#if d.status_code}
                    <span class:success-text={d.status_code < 300} class:error={d.status_code >= 400}>{d.status_code}</span>
                  {:else}
                    <span class="muted">—</span>
                  {/if}
                </td>
                <td class="muted detail">{d.error ?? ''}</td>
              </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>
    {/if}
  {/if}

  {/if}
</main>

{#if showWebhookModal}
<!-- svelte-ignore a11y_click_events_have_key_events a11y_interactive_supports_focus -->
<div class="modal-backdrop" on:click|self={() => showWebhookModal = false} role="dialog" aria-modal="true" tabindex="-1">
  <div class="modal webhook-modal">
    <h2>{editingWebhook ? 'Edit Webhook' : 'Add Webhook'}</h2>
    <form on:submit|preventDefault={saveWebhook}>
      <div class="form-row"><label for="wh-url">URL</label><input id="wh-url" bind:value={webhookForm.url} placeholder="https://example.com/hook" required /></div>
      <div class="form-row"><label for="wh-secret">Secret</label><input id="wh-secret" bind:value={webhookForm.secret} placeholder="Optional signing secret" /></div>
      <div class="form-row"><label for="wh-desc">Description</label><input id="wh-desc" bind:value={webhookForm.description} placeholder="Optional description" /></div>
      {#if editingWebhook}
      <div class="form-row">
        <label for="wh-enabled">Enabled</label>
        <input id="wh-enabled" type="checkbox" bind:checked={webhookForm.enabled} />
      </div>
      {/if}
      <div style="margin-top:0.75rem">
        <span class="form-label">Events</span>
        <div class="events-grid">
          {#each webhookEvents as ev}
            <label class="event-check">
              <input type="checkbox" checked={webhookForm.events.includes(ev)} on:change={() => toggleWebhookEvent(ev)} />
              <code>{ev}</code>
            </label>
          {/each}
        </div>
      </div>
      {#if webhookError}<p class="error" style="margin-top:0.5rem">{webhookError}</p>{/if}
      <div class="modal-actions">
        <button type="button" class="btn-xs" on:click={() => showWebhookModal = false}>Cancel</button>
        <button type="submit" class="btn-accent" disabled={webhookSaving}>{webhookSaving ? 'Saving…' : 'Save'}</button>
      </div>
    </form>
  </div>
</div>
{/if}

{#if showForceScanModal}
<!-- svelte-ignore a11y_click_events_have_key_events a11y_interactive_supports_focus -->
<div class="modal-backdrop" on:click|self={() => showForceScanModal = false} role="dialog" aria-modal="true" tabindex="-1">
  <div class="modal force-scan-modal">
    <div class="force-scan-icon">⚠️</div>
    <h2>Force Rescan Library?</h2>
    <p class="force-scan-desc">
      This will re-process <strong>every file</strong> in the library, overwriting existing track metadata.
      Unchanged files that are normally skipped will also be re-ingested.
    </p>
    <p class="force-scan-warn">
      This may take a long time depending on your library size and cannot be stopped once started.
    </p>
    <div class="modal-actions">
      <button class="btn-xs" on:click={() => showForceScanModal = false}>Cancel</button>
      <button class="btn-danger" on:click={startForceScan}>Yes, Force Rescan</button>
    </div>
  </div>
</div>
{/if}

{#if showForceAudiobookModal}
<!-- svelte-ignore a11y_click_events_have_key_events a11y_interactive_supports_focus -->
<div class="modal-backdrop" on:click|self={() => showForceAudiobookModal = false} role="dialog" aria-modal="true" tabindex="-1">
  <div class="modal force-scan-modal">
    <div class="force-scan-icon">⚠️</div>
    <h2>Re-ingest All Audiobooks?</h2>
    <p class="force-scan-desc">
      This will re-process <strong>every audiobook</strong> in your library and refresh metadata.
      Unchanged files that are normally skipped will also be re-ingested.
    </p>
    <p class="force-scan-warn">
      This may take a long time depending on your audiobook library size and cannot be stopped once started.
    </p>
    <div class="modal-actions">
      <button class="btn-xs" on:click={() => showForceAudiobookModal = false}>Cancel</button>
      <button class="btn-danger" on:click={startForceAudiobookScan}>Yes, Re-ingest Audiobooks</button>
    </div>
  </div>
</div>
{/if}

{#if showInviteModal}
<!-- svelte-ignore a11y_click_events_have_key_events a11y_interactive_supports_focus -->
<div class="modal-backdrop" on:click|self={() => showInviteModal = false} role="dialog" aria-modal="true" tabindex="-1">
  <div class="modal">
    <h2>Invite User</h2>
    {#if inviteResult}
      <p class="success">Invite created!</p>
      <label class="form-label" for="invite-url-input">Invite URL — share with the user:</label>
      <div class="copy-row">
        <input id="invite-url-input" readonly value={inviteResult.invite_url} class="copy-input" />
        <button class="btn-xs" on:click={() => navigator.clipboard.writeText(inviteResult!.invite_url)}>Copy</button>
      </div>
      <p class="muted" style="font-size:0.75rem;margin-top:0.4rem">Expires {new Date(inviteResult.expires_at).toLocaleDateString()}</p>
      <button class="btn-accent" style="margin-top:1rem;width:100%" on:click={() => { showInviteModal = false; inviteResult = null; }}>Done</button>
    {:else}
      <form on:submit|preventDefault={sendInvite}>
        <!-- svelte-ignore a11y_autofocus -->
        <label class="form-label" for="invite-email">Email address</label>
        <input id="invite-email" type="email" bind:value={inviteEmail} placeholder="user@example.com" autofocus required />
        {#if inviteError}<p class="error" style="margin-top:0.5rem">{inviteError}</p>{/if}
        <div class="modal-actions">
          <button type="button" class="btn-xs" on:click={() => showInviteModal = false}>Cancel</button>
          <button type="submit" class="btn-accent" disabled={inviteLoading}>{inviteLoading ? 'Creating…' : 'Create Invite'}</button>
        </div>
      </form>
    {/if}
  </div>
</div>
{/if}

<style>
  .admin-page { padding: 1.5rem 2rem; max-width: 1300px; margin: 0 auto; }
  .admin-header { display: flex; align-items: center; gap: 1.25rem; margin-bottom: 1.5rem; flex-wrap: wrap; }
  h1 { font-size: 1.4rem; font-weight: 700; color: var(--text-primary, #fff); margin: 0; flex-shrink: 0; }
  h2 { font-size: 1rem; font-weight: 600; margin: 0 0 1rem; color: var(--text-primary, #fff); }
  h3 { font-size: 0.875rem; font-weight: 600; margin: 0 0 0.75rem; color: var(--text-primary, #fff); }

  .tabs-scroll { flex: 1; min-width: 0; overflow-x: auto; -webkit-overflow-scrolling: touch; scrollbar-width: none; }
  .tabs-scroll::-webkit-scrollbar { display: none; }
  .tabs { display: flex; gap: 0.25rem; background: var(--surface, #1e1e2e); border-radius: 8px; padding: 3px; width: max-content; min-width: 100%; }
  .tab { background: transparent; border: none; color: var(--text-secondary, #888); padding: 0.4rem 0.85rem; border-radius: 6px; cursor: pointer; font-size: 0.85rem; transition: background 0.15s, color 0.15s; white-space: nowrap; }
  .tab:hover { background: var(--surface-hover, #2a2a3a); color: var(--text-primary, #fff); }
  .tab.active { background: var(--accent, #a78bfa); color: #fff; }

  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(110px, 1fr)); gap: 0.75rem; margin-bottom: 1.5rem; }
  .card { background: var(--surface, #1e1e2e); border-radius: 10px; padding: 1rem; display: flex; flex-direction: column; align-items: center; gap: 0.25rem; }
  .cv { font-size: 1.35rem; font-weight: 700; color: var(--accent, #a78bfa); }
  .cl { font-size: 0.68rem; color: var(--text-secondary, #888); text-transform: uppercase; letter-spacing: 0.05em; }

  .panel { background: var(--surface, #1e1e2e); border-radius: 10px; padding: 1.25rem; margin-bottom: 1.25rem; }
  .two-col { display: grid; grid-template-columns: 1fr 1fr; gap: 1.25rem; }
  @media (max-width: 700px) { .two-col { grid-template-columns: 1fr; } }

  .section-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 1rem; gap: 0.75rem; flex-wrap: wrap; }
  .section-header h2 { margin: 0; }

  .bar-chart { display: flex; align-items: flex-end; gap: 3px; height: 100px; padding-bottom: 1.4rem; }
  .bar-col { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: flex-end; height: 100%; position: relative; }
  .bar { width: 100%; background: var(--accent, #a78bfa); border-radius: 3px 3px 0 0; min-height: 2px; }
  .bar-date { position: absolute; bottom: -1.2rem; font-size: 0.6rem; color: var(--text-secondary, #888); white-space: nowrap; }

  .table-scroll { overflow-x: auto; -webkit-overflow-scrolling: touch; margin: 0 -0.25rem; padding: 0 0.25rem; }
  table { width: 100%; border-collapse: collapse; font-size: 0.85rem; }
  th { text-align: left; font-size: 0.68rem; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-secondary, #888); padding: 0.4rem 0.5rem; border-bottom: 1px solid var(--border, #333); white-space: nowrap; }
  td { padding: 0.42rem 0.5rem; border-bottom: 1px solid var(--border, #2a2a3a); color: var(--text-primary, #fff); }
  tr:last-child td { border-bottom: none; }
  tr.inactive td { opacity: 0.5; }
  .muted { color: var(--text-secondary, #888); }
  .plays { font-variant-numeric: tabular-nums; text-align: right; color: var(--accent, #a78bfa); font-weight: 600; }
  .nowrap { white-space: nowrap; }
  .detail { max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 0.75rem; }

  .btn-accent { background: var(--accent, #a78bfa); color: #fff; border: none; border-radius: 7px; padding: 0.42rem 1rem; font-size: 0.85rem; font-weight: 600; cursor: pointer; transition: opacity 0.15s; white-space: nowrap; }
  .btn-accent:hover { opacity: 0.85; }
  .btn-accent:disabled { opacity: 0.4; cursor: default; }
  .btn-xs { background: var(--surface-hover, #2a2a3a); border: 1px solid var(--border, #444); color: var(--text-secondary, #888); border-radius: 5px; padding: 0.15rem 0.5rem; font-size: 0.75rem; cursor: pointer; white-space: nowrap; }
  .btn-xs:hover { color: var(--text-primary, #fff); }
  .btn-xs:disabled { opacity: 0.4; cursor: default; }
  .btn-xs.danger { border-color: #f87171; color: #f87171; }
  .btn-xs.muted { color: var(--text-secondary, #666); }

  .toggle-btn { border: 1px solid var(--border, #444); background: transparent; color: var(--text-secondary, #888); border-radius: 6px; padding: 0.15rem 0.55rem; font-size: 0.75rem; cursor: pointer; transition: background 0.15s, color 0.15s; white-space: nowrap; }
  .toggle-btn:hover { background: var(--surface-hover, #2a2a3a); }
  .toggle-btn.active { background: var(--accent, #a78bfa); color: #fff; border-color: transparent; }

  .badge { font-size: 0.72rem; color: var(--text-secondary, #888); }
  .badge.accent { color: var(--accent, #a78bfa); }
  .verified-badge { color: #22c55e; font-weight: 700; }
  .unverified-badge { color: var(--text-secondary, #555); }
  .quota-edit { display: flex; align-items: center; gap: 0.25rem; }
  .quota-input { width: 72px; padding: 0.1rem 0.35rem; background: var(--surface-hover, #2a2a3a); border: 1px solid var(--border, #444); border-radius: 4px; color: var(--text-primary, #fff); font-size: 0.75rem; }
  .delete-confirm { display: flex; align-items: center; gap: 0.25rem; font-size: 0.75rem; }

  .progress-bar-wrap { background: var(--surface-hover, #2a2a3a); border-radius: 4px; height: 8px; margin-bottom: 0.5rem; overflow: hidden; }
  .progress-bar { height: 100%; background: var(--accent, #a78bfa); border-radius: 4px; transition: width 0.3s; }
  .progress-info { font-size: 0.8rem; margin-bottom: 0.75rem; }
  .ingest-log { background: #0a0a14; border-radius: 6px; padding: 0.75rem; max-height: 180px; overflow-y: auto; font-family: monospace; font-size: 0.72rem; color: var(--text-secondary, #888); }
  .log-line { padding: 1px 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

  .settings-form { display: flex; flex-direction: column; gap: 0.7rem; max-width: 500px; }
  .form-row { display: grid; grid-template-columns: 140px 1fr; align-items: center; gap: 0.75rem; }
  .form-row label { font-size: 0.84rem; color: var(--text-secondary, #888); }
  .form-row input:not([type="checkbox"]) { padding: 0.4rem 0.6rem; background: var(--surface-hover, #2a2a3a); border: 1px solid var(--border, #444); border-radius: 6px; color: var(--text-primary, #fff); font-size: 0.85rem; }
  .form-row input:focus { outline: none; border-color: var(--accent, #a78bfa); }
  .form-label { font-size: 0.82rem; color: var(--text-secondary, #888); margin-bottom: 0.3rem; display: block; }
  .test-row { display: flex; gap: 0.5rem; flex-wrap: wrap; }
  .test-row input { flex: 1; min-width: 180px; padding: 0.4rem 0.6rem; background: var(--surface-hover, #2a2a3a); border: 1px solid var(--border, #444); border-radius: 6px; color: var(--text-primary, #fff); font-size: 0.85rem; }
  .test-row input:focus { outline: none; border-color: var(--accent, #a78bfa); }

  .pagination { display: flex; align-items: center; gap: 1rem; justify-content: center; margin-top: 1rem; font-size: 0.82rem; flex-wrap: wrap; }
  .pagination button { background: var(--surface-hover, #2a2a3a); border: 1px solid var(--border, #444); color: var(--text-primary, #fff); border-radius: 5px; padding: 0.3rem 0.75rem; cursor: pointer; }
  .pagination button:disabled { opacity: 0.35; cursor: default; }

  .modal-backdrop { position: fixed; inset: 0; background: rgba(0,0,0,0.6); display: flex; align-items: center; justify-content: center; z-index: 100; padding: 1rem; }
  .modal { background: var(--surface, #1e1e2e); border-radius: 12px; padding: 1.5rem; width: min(420px, 100%); box-sizing: border-box; }
  .modal h2 { margin: 0 0 1.25rem; font-size: 1.1rem; color: var(--text-primary, #fff); }
  .modal input[type="email"] { width: 100%; padding: 0.45rem 0.7rem; background: var(--surface-hover, #2a2a3a); border: 1px solid var(--border, #444); border-radius: 6px; color: var(--text-primary, #fff); font-size: 0.9rem; box-sizing: border-box; }
  .modal input[type="email"]:focus { outline: none; border-color: var(--accent, #a78bfa); }
  .modal-actions { display: flex; justify-content: flex-end; gap: 0.75rem; margin-top: 1.25rem; }
  .copy-row { display: flex; gap: 0.5rem; margin-top: 0.4rem; }
  .copy-input { flex: 1; padding: 0.35rem 0.6rem; background: var(--surface-hover, #2a2a3a); border: 1px solid var(--border, #444); border-radius: 6px; color: var(--text-secondary, #888); font-size: 0.8rem; min-width: 0; }

  .error { color: #f87171; font-size: 0.85rem; }
  .success { color: #34d399; font-size: 0.85rem; }
  .success-text { color: #34d399; }
  code { font-family: monospace; font-size: 0.8rem; background: var(--surface-hover, #2a2a3a); padding: 1px 5px; border-radius: 3px; }

  .scan-split-btn { display: flex; align-items: stretch; border-radius: 7px; overflow: hidden; }
  .scan-main { border-radius: 0; border-top-left-radius: 7px; border-bottom-left-radius: 7px; border-right: 1px solid rgba(255,255,255,0.2); }
  .scan-arrow { border-radius: 0; border-top-right-radius: 7px; border-bottom-right-radius: 7px; padding: 0.42rem 0.6rem; font-size: 0.7rem; min-width: 28px; }
  .scan-arrow:hover:not(:disabled) { opacity: 0.85; }

  .force-scan-modal { width: min(440px, 100%); text-align: center; }
  .force-scan-icon { font-size: 2.5rem; margin-bottom: 0.75rem; }
  .force-scan-modal h2 { margin: 0 0 1rem; }
  .force-scan-desc { font-size: 0.875rem; color: var(--text-primary, #fff); margin: 0 0 0.75rem; line-height: 1.5; }
  .force-scan-warn { font-size: 0.8rem; color: #f87171; margin: 0 0 1.25rem; }
  .btn-danger { background: #dc2626; color: #fff; border: none; border-radius: 7px; padding: 0.42rem 1rem; font-size: 0.85rem; font-weight: 600; cursor: pointer; transition: opacity 0.15s; white-space: nowrap; }
  .btn-danger:hover { opacity: 0.85; }

  .wh-url { font-family: monospace; font-size: 0.78rem; max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .row-actions { display: flex; gap: 0.3rem; flex-wrap: nowrap; }
  .events-grid { display: flex; flex-direction: column; gap: 0.4rem; max-height: 220px; overflow-y: auto; padding: 0.5rem; background: var(--surface-hover, #2a2a3a); border-radius: 6px; }
  .event-check { display: flex; align-items: center; gap: 0.5rem; cursor: pointer; font-size: 0.82rem; color: var(--text-secondary, #888); }
  .event-check input { accent-color: var(--accent, #a78bfa); }
  .webhook-modal { width: min(540px, 100%); }

  @media (max-width: 640px) {
    .admin-page { padding: 1rem; }
    .admin-header { gap: 0.75rem; margin-bottom: 1.25rem; }
    .panel { padding: 1rem; }
    .form-row { grid-template-columns: 1fr; gap: 0.25rem; }
    .form-row label { font-size: 0.78rem; }
    .settings-form { max-width: 100%; }
  }
</style>

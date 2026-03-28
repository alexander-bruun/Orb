<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { authStore } from '$lib/stores/auth';
  import { social } from '$lib/api/social';
  import { getApiBase } from '$lib/api/base';
  import type { PublicProfile, ActivityRow, UserStats } from '$lib/api/social';
  import type { Playlist } from '$lib/types';

  const username = $page.params.username;
  $: isOwnProfile = $authStore.user?.username === username;

  let profile: PublicProfile | null = null;
  let isFollowing = false;
  let playlists: Playlist[] = [];
  let activity: ActivityRow[] = [];
  let stats: UserStats | null = null;
  let activeTab: 'activity' | 'playlists' | 'stats' = 'activity';
  let loading = true;
  let error = '';
  let followLoading = false;

  function avatarUrl(key: string | null | undefined): string | null {
    if (!key) return null;
    // key is like "avatars/uuid.jpg" — serve via /covers/avatar/{filename}
    const filename = key.split('/').pop();
    return filename ? `${getApiBase()}/covers/avatar/${filename}` : null;
  }

  function formatDate(iso: string): string {
    return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'long' });
  }

  function formatDuration(ms: number): string {
    const h = Math.floor(ms / 3_600_000);
    const m = Math.floor((ms % 3_600_000) / 60_000);
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
  }

  function activityLabel(row: ActivityRow): string {
    const name = row.display_name || row.username;
    const meta = row.metadata ?? {};
    switch (row.type) {
      case 'play':
        return `${name} played ${meta.track_name ?? 'a track'}`;
      case 'favorite':
        return `${name} favorited ${meta.track_name ?? 'a track'}`;
      case 'playlist_create':
        return `${name} created playlist "${meta.playlist_name ?? ''}"`;
      case 'playlist_follow':
        return `${name} joined a collaborative playlist`;
      default:
        return `${name} did something`;
    }
  }

  async function loadProfile() {
    loading = true;
    error = '';
    try {
      const res = await social.getProfile(username);
      profile = res.profile;
      isFollowing = res.is_following ?? false;
    } catch (e: any) {
      error = e?.message ?? 'Profile not found or not public.';
    } finally {
      loading = false;
    }
  }

  async function loadTab(tab: typeof activeTab) {
    activeTab = tab;
    if (!profile) return;
    if (tab === 'playlists' && playlists.length === 0) {
      playlists = await social.getProfilePlaylists(username).catch(() => []);
    }
    if (tab === 'activity' && activity.length === 0) {
      activity = await social.getProfileActivity(username).catch(() => []);
    }
    if (tab === 'stats' && !stats) {
      stats = await social.getProfileStats(username).catch(() => null);
    }
  }

  async function toggleFollow() {
    if (!profile || followLoading) return;
    followLoading = true;
    try {
      if (isFollowing) {
        await social.unfollow(username);
        isFollowing = false;
        profile = { ...profile, follower_count: profile.follower_count - 1 };
      } else {
        await social.follow(username);
        isFollowing = true;
        profile = { ...profile, follower_count: profile.follower_count + 1 };
      }
    } catch {
      // silently ignore
    } finally {
      followLoading = false;
    }
  }

  onMount(async () => {
    await loadProfile();
    if (profile) {
      activity = await social.getProfileActivity(username).catch(() => []);
    }
  });
</script>

<svelte:head>
  <title>{profile?.display_name || username} · Orb</title>
</svelte:head>

<div class="profile-page">
  {#if loading}
    <div class="state-msg">Loading…</div>

  {:else if error}
    <div class="state-msg state-msg--error">{error}</div>

  {:else if profile}
    <!-- Header -->
    <div class="profile-header">
      <div class="avatar-wrap">
        {#if avatarUrl(profile.avatar_key)}
          <img src={avatarUrl(profile.avatar_key)} alt="" class="avatar" />
        {:else}
          <div class="avatar avatar--initials">
            {(profile.display_name || profile.username).slice(0, 2).toUpperCase()}
          </div>
        {/if}
      </div>

      <div class="profile-meta">
        <h1 class="display-name">{profile.display_name || profile.username}</h1>
        {#if profile.display_name}
          <span class="username">@{profile.username}</span>
        {/if}
        {#if profile.bio}
          <p class="bio">{profile.bio}</p>
        {/if}

        <div class="stats-row">
          <span class="stat"><strong>{profile.follower_count}</strong> followers</span>
          <span class="stat"><strong>{profile.following_count}</strong> following</span>
          <span class="stat"><strong>{profile.playlist_count}</strong> playlists</span>
          <span class="stat muted">Joined {formatDate(profile.joined_at)}</span>
        </div>

        <div class="actions">
          {#if isOwnProfile}
            <a href="/settings#profile" class="btn-secondary">Edit profile</a>
          {:else if $authStore.user}
            <button
              class="btn-primary"
              class:btn-secondary={isFollowing}
              on:click={toggleFollow}
              disabled={followLoading}
            >
              {isFollowing ? 'Unfollow' : 'Follow'}
            </button>
          {/if}
        </div>
      </div>
    </div>

    <!-- Tabs -->
    <div class="tabs">
      <button class="tab" class:active={activeTab === 'activity'} on:click={() => loadTab('activity')}>Activity</button>
      <button class="tab" class:active={activeTab === 'playlists'} on:click={() => loadTab('playlists')}>Playlists</button>
      <button class="tab" class:active={activeTab === 'stats'} on:click={() => loadTab('stats')}>Stats</button>
    </div>

    <!-- Tab content -->
    <div class="tab-content">
      {#if activeTab === 'activity'}
        {#if activity.length === 0}
          <p class="empty">No recent activity.</p>
        {:else}
          <ul class="activity-list">
            {#each activity as row (row.id)}
              <li class="activity-item">
                <div class="activity-avatar">
                  {#if row.avatar_key}
                    <img src={avatarUrl(row.avatar_key)} alt="" />
                  {:else}
                    <div class="activity-avatar--initials">
                      {(row.display_name || row.username).slice(0, 1).toUpperCase()}
                    </div>
                  {/if}
                </div>
                <div class="activity-body">
                  <span class="activity-text">{activityLabel(row)}</span>
                  {#if row.metadata?.cover_key}
                    <!-- nothing for now -->
                  {/if}
                  <span class="activity-time">{formatDate(row.created_at)}</span>
                </div>
              </li>
            {/each}
          </ul>
        {/if}

      {:else if activeTab === 'playlists'}
        {#if playlists.length === 0}
          <p class="empty">No public playlists.</p>
        {:else}
          <ul class="playlist-list">
            {#each playlists as pl (pl.id)}
              <li>
                <a href="/playlists/{pl.id}" class="playlist-item">
                  <div class="playlist-art">
                    <img src="{getApiBase()}/covers/playlist/{pl.id}" alt="" on:error={(e) => (e.currentTarget as HTMLImageElement).style.display='none'} />
                  </div>
                  <span class="playlist-name">{pl.name}</span>
                </a>
              </li>
            {/each}
          </ul>
        {/if}

      {:else if activeTab === 'stats'}
        {#if !stats}
          <p class="empty">No stats available.</p>
        {:else}
          <div class="stats-grid">
            <div class="stat-card">
              <div class="stat-value">{stats.total_plays.toLocaleString()}</div>
              <div class="stat-label">Total plays</div>
            </div>
            <div class="stat-card">
              <div class="stat-value">{formatDuration(stats.total_played_ms)}</div>
              <div class="stat-label">Total listening time</div>
            </div>
          </div>

          {#if stats.top_artists.length > 0}
            <h3 class="sub-heading">Top artists</h3>
            <ol class="top-artists">
              {#each stats.top_artists as a, i}
                <li class="top-artist-row">
                  <span class="rank">{i + 1}</span>
                  <a href="/artists/{a.artist_id}" class="artist-name">{a.artist_name}</a>
                  <span class="play-count">{a.plays} plays</span>
                </li>
              {/each}
            </ol>
          {/if}
        {/if}
      {/if}
    </div>
  {/if}
</div>

<style>
  .profile-page {
    max-width: 720px;
    margin: 0 auto;
    padding: 32px 16px 80px;
  }

  .state-msg {
    text-align: center;
    color: var(--text-muted);
    padding: 64px 0;
    font-size: 0.9rem;
  }
  .state-msg--error { color: var(--error, #ef4444); }

  /* Header */
  .profile-header {
    display: flex;
    gap: 24px;
    align-items: flex-start;
    margin-bottom: 32px;
  }

  .avatar-wrap { flex-shrink: 0; }

  .avatar {
    width: 96px;
    height: 96px;
    border-radius: 50%;
    object-fit: cover;
    background: var(--bg-elevated);
  }
  .avatar--initials {
    width: 96px;
    height: 96px;
    border-radius: 50%;
    background: var(--accent-dim);
    color: var(--accent);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 1.5rem;
    font-weight: 600;
  }

  .profile-meta { flex: 1; min-width: 0; }

  .display-name {
    font-size: 1.5rem;
    font-weight: 700;
    margin: 0 0 2px;
    color: var(--text);
  }
  .username {
    font-size: 0.85rem;
    color: var(--text-muted);
    display: block;
    margin-bottom: 6px;
  }
  .bio {
    font-size: 0.875rem;
    color: var(--text-muted);
    margin: 0 0 12px;
    white-space: pre-wrap;
  }

  .stats-row {
    display: flex;
    flex-wrap: wrap;
    gap: 16px;
    margin-bottom: 16px;
    font-size: 0.85rem;
    color: var(--text-muted);
  }
  .stat strong { color: var(--text); }
  .muted { color: var(--text-muted); }

  .actions { display: flex; gap: 8px; }

  .btn-primary {
    padding: 7px 18px;
    border-radius: 6px;
    background: var(--accent);
    color: #000;
    font-size: 0.875rem;
    font-weight: 600;
    border: none;
    cursor: pointer;
    transition: opacity 0.15s;
  }
  .btn-primary:hover { opacity: 0.85; }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }

  .btn-secondary {
    padding: 7px 18px;
    border-radius: 6px;
    background: var(--bg-elevated);
    color: var(--text);
    font-size: 0.875rem;
    font-weight: 500;
    border: 1px solid var(--border);
    cursor: pointer;
    text-decoration: none;
    transition: background 0.15s;
  }
  .btn-secondary:hover { background: var(--bg-hover); }

  /* Tabs */
  .tabs {
    display: flex;
    gap: 4px;
    border-bottom: 1px solid var(--border);
    margin-bottom: 24px;
  }
  .tab {
    padding: 8px 16px;
    font-size: 0.875rem;
    font-weight: 500;
    color: var(--text-muted);
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    cursor: pointer;
    transition: color 0.15s, border-color 0.15s;
    margin-bottom: -1px;
  }
  .tab:hover { color: var(--text); }
  .tab.active { color: var(--accent); border-bottom-color: var(--accent); }

  .empty {
    color: var(--text-muted);
    font-size: 0.875rem;
    padding: 24px 0;
  }

  /* Activity */
  .activity-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 12px; }
  .activity-item { display: flex; gap: 12px; align-items: flex-start; }
  .activity-avatar img,
  .activity-avatar--initials {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    object-fit: cover;
    flex-shrink: 0;
  }
  .activity-avatar--initials {
    background: var(--bg-elevated);
    color: var(--text-muted);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.75rem;
    font-weight: 600;
  }
  .activity-body { display: flex; flex-direction: column; gap: 2px; }
  .activity-text { font-size: 0.875rem; color: var(--text); }
  .activity-time { font-size: 0.75rem; color: var(--text-muted); }

  /* Playlists */
  .playlist-list { list-style: none; padding: 0; margin: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 16px; }
  .playlist-item { display: flex; flex-direction: column; gap: 8px; text-decoration: none; color: var(--text); }
  .playlist-art { aspect-ratio: 1; border-radius: 8px; overflow: hidden; background: var(--bg-elevated); }
  .playlist-art img { width: 100%; height: 100%; object-fit: cover; }
  .playlist-name { font-size: 0.875rem; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

  /* Stats */
  .stats-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-bottom: 24px; }
  .stat-card { background: var(--bg-elevated); border-radius: 10px; padding: 16px; }
  .stat-value { font-size: 1.5rem; font-weight: 700; color: var(--text); }
  .stat-label { font-size: 0.8rem; color: var(--text-muted); margin-top: 4px; }

  .sub-heading { font-size: 0.875rem; font-weight: 600; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.05em; margin: 0 0 12px; }
  .top-artists { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 8px; }
  .top-artist-row { display: flex; align-items: center; gap: 12px; font-size: 0.875rem; }
  .rank { width: 20px; text-align: right; color: var(--text-muted); font-size: 0.75rem; }
  .artist-name { flex: 1; color: var(--text); text-decoration: none; }
  .artist-name:hover { color: var(--accent); }
  .play-count { color: var(--text-muted); }

  @media (max-width: 480px) {
    .profile-header { flex-direction: column; align-items: center; text-align: center; }
    .stats-row { justify-content: center; }
    .actions { justify-content: center; }
    .stats-grid { grid-template-columns: 1fr; }
  }
</style>

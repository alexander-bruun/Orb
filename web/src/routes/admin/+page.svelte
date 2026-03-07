<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { authStore } from '$lib/stores/auth';
  import { admin as adminApi } from '$lib/api/admin';
  import type {
    AdminSummary,
    UserPlayStat,
    TrackPlayCount,
    ArtistPlayCount,
    DailyPlayCount
  } from '$lib/api/admin';

  let summary: AdminSummary | null = null;
  let users: UserPlayStat[] = [];
  let topTracks: TrackPlayCount[] = [];
  let topArtists: ArtistPlayCount[] = [];
  let playsByDay: DailyPlayCount[] = [];
  let loading = true;
  let error = '';

  onMount(async () => {
    if (!$authStore.user?.is_admin) {
      goto('/');
      return;
    }
    try {
      [summary, users, topTracks, topArtists, playsByDay] = await Promise.all([
        adminApi.summary(),
        adminApi.users(),
        adminApi.topTracks(10),
        adminApi.topArtists(10),
        adminApi.playsByDay(30)
      ]);
    } catch (e: any) {
      error = e.message ?? 'Failed to load analytics';
    } finally {
      loading = false;
    }
  });

  async function toggleAdmin(user: UserPlayStat) {
    try {
      await adminApi.setUserAdmin(user.user_id, !user.is_admin);
      users = users.map(u =>
        u.user_id === user.user_id ? { ...u, is_admin: !u.is_admin } : u
      );
    } catch (e: any) {
      alert(e.message ?? 'Failed to update user');
    }
  }

  // Bar chart helpers
  function maxPlays(data: DailyPlayCount[]): number {
    return Math.max(1, ...data.map(d => d.plays));
  }

  function fmtMs(ms: number): string {
    const h = Math.floor(ms / 3_600_000);
    const m = Math.floor((ms % 3_600_000) / 60_000);
    return h > 0 ? `${h}h ${m}m` : `${m}m`;
  }
</script>

<svelte:head>
  <title>Admin — Orb</title>
</svelte:head>

<main class="admin-page">
  <h1>Admin Dashboard</h1>

  {#if loading}
    <p class="muted">Loading analytics…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else}

    <!-- Summary cards -->
    {#if summary}
    <section class="cards">
      <div class="card">
        <span class="card-value">{summary.total_users}</span>
        <span class="card-label">Users</span>
      </div>
      <div class="card">
        <span class="card-value">{summary.total_tracks.toLocaleString()}</span>
        <span class="card-label">Tracks</span>
      </div>
      <div class="card">
        <span class="card-value">{summary.total_albums.toLocaleString()}</span>
        <span class="card-label">Albums</span>
      </div>
      <div class="card">
        <span class="card-value">{summary.total_artists.toLocaleString()}</span>
        <span class="card-label">Artists</span>
      </div>
      <div class="card">
        <span class="card-value">{summary.total_plays.toLocaleString()}</span>
        <span class="card-label">Total Plays</span>
      </div>
      <div class="card">
        <span class="card-value">{fmtMs(summary.total_played_ms)}</span>
        <span class="card-label">Listened</span>
      </div>
    </section>
    {/if}

    <!-- Plays-by-day chart -->
    <section class="panel">
      <h2>Plays — last 30 days</h2>
      <div class="bar-chart">
        {#each playsByDay as day}
          {@const pct = (day.plays / maxPlays(playsByDay)) * 100}
          <div class="bar-col" title="{day.date}: {day.plays} plays">
            <div class="bar" style="height:{pct}%"></div>
            <!-- show label only every 5 days to avoid crowding -->
            {#if Number(day.date.slice(-2)) % 5 === 1}
              <span class="bar-date">{day.date.slice(5)}</span>
            {/if}
          </div>
        {/each}
      </div>
    </section>

    <div class="two-col">
      <!-- Top tracks -->
      <section class="panel">
        <h2>Top Tracks</h2>
        <table>
          <thead><tr><th>#</th><th>Title</th><th>Artist</th><th>Plays</th></tr></thead>
          <tbody>
            {#each topTracks as t, i}
              <tr>
                <td class="muted">{i + 1}</td>
                <td>{t.title}</td>
                <td class="muted">{t.artist_name ?? '—'}</td>
                <td class="plays">{t.plays}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </section>

      <!-- Top artists -->
      <section class="panel">
        <h2>Top Artists</h2>
        <table>
          <thead><tr><th>#</th><th>Artist</th><th>Plays</th></tr></thead>
          <tbody>
            {#each topArtists as a, i}
              <tr>
                <td class="muted">{i + 1}</td>
                <td>{a.name}</td>
                <td class="plays">{a.plays}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </section>
    </div>

    <!-- Users -->
    <section class="panel">
      <h2>Users</h2>
      <table>
        <thead>
          <tr>
            <th>Username</th>
            <th>Email</th>
            <th>Plays</th>
            <th>Joined</th>
            <th>Admin</th>
          </tr>
        </thead>
        <tbody>
          {#each users as u}
            <tr>
              <td>{u.username}</td>
              <td class="muted">{u.email}</td>
              <td class="plays">{u.play_count}</td>
              <td class="muted">{new Date(u.created_at).toLocaleDateString()}</td>
              <td>
                <!-- Prevent self-demotion -->
                {#if u.user_id !== $authStore.user?.id}
                  <button
                    class="toggle-admin"
                    class:active={u.is_admin}
                    on:click={() => toggleAdmin(u)}
                    title={u.is_admin ? 'Remove admin' : 'Make admin'}
                  >
                    {u.is_admin ? 'Admin' : 'User'}
                  </button>
                {:else}
                  <span class="badge-admin">You (admin)</span>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </section>

  {/if}
</main>

<style>
  .admin-page {
    padding: 2rem;
    max-width: 1200px;
    margin: 0 auto;
  }

  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin-bottom: 1.5rem;
    color: var(--text-primary, #fff);
  }

  h2 {
    font-size: 1rem;
    font-weight: 600;
    margin-bottom: 1rem;
    color: var(--text-primary, #fff);
  }

  /* Summary cards */
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 1rem;
    margin-bottom: 2rem;
  }

  .card {
    background: var(--surface, #1e1e2e);
    border-radius: 10px;
    padding: 1.2rem 1rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.3rem;
  }

  .card-value {
    font-size: 1.5rem;
    font-weight: 700;
    color: var(--accent, #a78bfa);
  }

  .card-label {
    font-size: 0.75rem;
    color: var(--text-secondary, #888);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  /* Panels */
  .panel {
    background: var(--surface, #1e1e2e);
    border-radius: 10px;
    padding: 1.4rem;
    margin-bottom: 1.5rem;
  }

  .two-col {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1.5rem;
  }

  @media (max-width: 700px) {
    .two-col { grid-template-columns: 1fr; }
  }

  /* Bar chart */
  .bar-chart {
    display: flex;
    align-items: flex-end;
    gap: 3px;
    height: 120px;
    padding-bottom: 1.4rem;
    position: relative;
  }

  .bar-col {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: flex-end;
    height: 100%;
    position: relative;
  }

  .bar {
    width: 100%;
    background: var(--accent, #a78bfa);
    border-radius: 3px 3px 0 0;
    min-height: 2px;
    transition: height 0.3s ease;
  }

  .bar-date {
    position: absolute;
    bottom: -1.3rem;
    font-size: 0.6rem;
    color: var(--text-secondary, #888);
    white-space: nowrap;
  }

  /* Tables */
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.875rem;
  }

  th {
    text-align: left;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-secondary, #888);
    padding: 0.4rem 0.6rem;
    border-bottom: 1px solid var(--border, #333);
  }

  td {
    padding: 0.5rem 0.6rem;
    border-bottom: 1px solid var(--border, #2a2a3a);
    color: var(--text-primary, #fff);
  }

  tr:last-child td { border-bottom: none; }

  .muted { color: var(--text-secondary, #888); }

  .plays {
    font-variant-numeric: tabular-nums;
    text-align: right;
    color: var(--accent, #a78bfa);
    font-weight: 600;
  }

  .toggle-admin {
    border: 1px solid var(--border, #444);
    background: transparent;
    color: var(--text-secondary, #888);
    border-radius: 6px;
    padding: 0.2rem 0.7rem;
    font-size: 0.75rem;
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }

  .toggle-admin:hover { background: var(--surface-hover, #2a2a3a); }

  .toggle-admin.active {
    background: var(--accent, #a78bfa);
    color: #fff;
    border-color: transparent;
  }

  .badge-admin {
    font-size: 0.75rem;
    color: var(--accent, #a78bfa);
  }

  .error { color: #f87171; }
</style>

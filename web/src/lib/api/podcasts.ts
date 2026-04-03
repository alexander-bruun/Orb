import { apiFetch } from './client';
import type { Podcast, PodcastEpisode, PodcastEpisodeProgress } from '../types';

export const podcasts = {
  list: (limit = 50, offset = 0): Promise<{ podcasts: Podcast[] }> =>
    apiFetch(`/podcasts?limit=${limit}&offset=${offset}`),

  getSubscriptions: (limit = 50, offset = 0): Promise<{ podcasts: Podcast[] }> =>
    apiFetch(`/podcasts/subscriptions?limit=${limit}&offset=${offset}`),

  recentlyAdded: (limit = 20): Promise<{ podcasts: Podcast[] }> =>
    apiFetch(`/podcasts/recently-added?limit=${limit}`),

  withNewEpisodes: (limit = 20): Promise<{ podcasts: Podcast[] }> =>
    apiFetch(`/podcasts/with-new-episodes?limit=${limit}`),

  subscribe: (url: string): Promise<{ podcast: Podcast }> =>
    apiFetch('/podcasts/subscribe', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url }),
    }),

  get: (id: string): Promise<{ podcast: Podcast }> =>
    apiFetch(`/podcasts/${id}`),

  update: (id: string, data: Partial<Podcast>): Promise<void> =>
    apiFetch(`/podcasts/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    }),

  delete: (id: string): Promise<void> =>
    apiFetch(`/podcasts/${id}`, { method: 'DELETE' }),

  listEpisodes: (
    id: string,
    limit = 50,
    offset = 0,
    search = "",
    sortBy = "pub_date",
    sortDir = "desc",
  ): Promise<{ episodes: PodcastEpisode[]; total: number }> => {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    if (search) params.set("search", search);
    if (sortBy !== "pub_date") params.set("sort_by", sortBy);
    if (sortDir !== "desc") params.set("sort_dir", sortDir);
    return apiFetch(`/podcasts/${id}/episodes?${params}`);
  },

  unsubscribe: (id: string): Promise<void> =>
    apiFetch(`/podcasts/${id}/unsubscribe`, { method: 'DELETE' }),

  getInProgress: (limit = 20): Promise<{ episodes: PodcastEpisode[] }> =>
    apiFetch(`/podcasts/episodes/in-progress?limit=${limit}`),

  getEpisode: (id: string): Promise<{ episode: PodcastEpisode }> =>
    apiFetch(`/podcasts/episodes/${id}`),

  getProgress: (id: string): Promise<{ progress: PodcastEpisodeProgress }> =>
    apiFetch(`/podcasts/episodes/${id}/progress`),

  updateProgress: (id: string, positionMs: number, completed: boolean): Promise<void> =>
    apiFetch(`/podcasts/episodes/${id}/progress`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ position_ms: positionMs, completed }),
    }),

  download: (id: string): Promise<void> =>
    apiFetch(`/podcasts/episodes/${id}/download`, { method: 'POST' }),
};

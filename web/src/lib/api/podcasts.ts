import { apiFetch } from './client';
import type { Podcast, PodcastEpisode, PodcastEpisodeProgress } from '../types';

export const podcasts = {
  list: (): Promise<{ podcasts: Podcast[] }> =>
    apiFetch('/podcasts'),

  getSubscriptions: (): Promise<{ podcasts: Podcast[] }> =>
    apiFetch('/podcasts/subscriptions'),

  subscribe: (url: string): Promise<{ podcast: Podcast }> =>
    apiFetch('/podcasts/subscribe', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url }),
    }),

  get: (id: string): Promise<{ podcast: Podcast }> =>
    apiFetch(`/podcasts/${id}`),

  listEpisodes: (id: string, limit = 50, offset = 0): Promise<{ episodes: PodcastEpisode[] }> =>
    apiFetch(`/podcasts/${id}/episodes?limit=${limit}&offset=${offset}`),

  unsubscribe: (id: string): Promise<void> =>
    apiFetch(`/podcasts/${id}`, { method: 'DELETE' }),

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

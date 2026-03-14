# Feature Ideas

- "Radio" mode — infinite queue seeded from a track or artist using the existing `recommend` service, auto-fetching more tracks as the queue drains.
- Add native make targets for building to Windows, Linux and Mac like Android and iOS so i dont have to rely on pipeline built binaries locally for testing.
- Auto-playlist (smart playlist) — saved filter rules (genre = "Jazz", year > 2000, play count < 5) that dynamically populate a playlist on open. These should be shown on the home page, and refresh once a week, and once a day when it makes sense.
- Create a script that cleans up build artifacts - maybe the files that fall under .gitignore?
- Integrations: webhooks for events & notifications, add a new tab to the admin ui where they can be configured and setup.
- iOS Support: Start developing iOS app using Tauri v2
- Make the admin UI responsive and good looking on mobile.
- On-the-fly transcoding / bitrate limiting for bandwidth-constrained devices based on the users settings.
- Playlist collaboration — allow other users on the same instance to add/remove tracks from a shared playlist (extends the existing listen party infrastructure). Collaborative playlists and shareable invite links with permissions
- Rework web browser tab titles, the tab title doesnt update based on what page the user is on.
- Track ratings (1–5 stars) — finer-grained than a binary favorite; enables weighted recommendations. Add a star in addition to favorite button, that shows 5 stars popping out of the star around it, then the user can put a rating from 1-5.
- Rework the admin settings base url depending on the platform, if on mobile it is aware of this due to the setup. On the website it should use the domain of the website. This way the register url that is sent out should work properly.
- Favorites track numbers should not be from the album index, but the position in the favorite list.
- Implement mail verification, and then and a verified badge to the user page if it is verified. And if the verify mail didnt arrive, we should implement retry logic so the user can try again.
- Look into why ingest doesn't fall back to polling for new albums / tracks when fsnotify is not an available feature.

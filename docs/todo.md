# Feature Ideas

- "Radio" mode — infinite queue seeded from a track or artist using the existing `recommend` service, auto-fetching more tracks as the queue drains.
- Add native make targets for building to Windows, Linux and Mac like Android and iOS.
- Auto-playlist (smart playlist) — saved filter rules (genre = "Jazz", year > 2000, play count < 5) that dynamically populate a playlist on open. These should be shown on the home page, and refresh once a week, and once a day when it makes sense.
- Create a script that cleans up build artifacts - maybe the files that fall under .gitignore?
- If i am on the library page, and ingest adds a new album, the page should be dynamically updated to show the new album without refreshing.
- If i favorite a track, and i have the favorite page open, it should dynamically add the track.
- Integrations: webhooks for events & notifications.
- iOS Support: Start developing iOS app using Tauri v2
- Make the admin UI responsive and good looking on mobile.
- On mobile when no last played track is present, instead of showin a empty island, dont show the island at all.
- On mobile when pressing the cast button it should open the native connected devices / bluetooth settings page on android.
- On-the-fly transcoding / bitrate limiting for bandwidth-constrained devices based on the users settings.
- Playlist collaboration — allow other users on the same instance to add/remove tracks from a shared playlist (extends the existing listen party infrastructure). Collaborative playlists and shareable invite links with permissions
- Rework web browser tab titles, the tab title doesnt update based on what page the user is on.
- Track ratings (1–5 stars) — finer-grained than a binary favorite; enables weighted recommendations. Add a star in addition to favorite button, that shows 5 stars popping out of the star around it, then the user can put a rating from 1-5.
- Update the github release ci to work with the new project structure and the changes in the makefile.

# TODO: GOGOGO!

Artist should also sort by characters like album title. Currently it just says #.
Year should show the numerical years in the right hand side. Currently it just says #
Remove sort by labe, i dont care for that.

When there is no current playing song, or one from the local storage from a previous play. Then add a skeleton placeholder for the image and titles.

Add 3 dots to the current playing song if its visible in the current page view, they should be jumping one at a time sequentially to show activity.

Rework the auth system, the token keeps getting invalidated, and even though im still visually logged in i have to log out and back in to see things.

Make the playback engine react to media controls from keyboard keys (pause play previous next).

If there is no song left in the personal queue, and its a song in the middle of a playlist or album where it was original started from, then playback should continue to the next song while taking shuffle mode into account.

Remember the state of the cover size. Large / small in the local browser storage so it persists across refreshes.

Add shuffle button to the artist page and playlist pages.

Update search frontend to show search results, and update the backend to search for both artists and albums at the same time, and show both in the search results.

Implement sound visualizers that are toggleable.

Implement OpenLyrics for lyrics synced to the playing music, shown in a toggleable modal.

Implement metadata retrieval from something like musicbrainz.

Implement algorithm for suggested similar tracks. This should be used for user specific generated playlists based on their listen history. We should also use it for when a album or playlist runs out of tracks to play, afterwards recommended tracks should start playing.

package com.orb.app

import android.util.Log
import org.json.JSONArray
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

/**
 * Lightweight HTTP client for fetching browse data from the Orb API.
 * Used by MediaLibraryService to populate the Android Auto browse tree.
 */
class OrbApiClient(
    private var baseUrl: String,
    private var token: String
) {
    companion object {
        private const val TAG = "OrbApiClient"
        private const val TIMEOUT = 8000
    }

    fun updateCredentials(baseUrl: String, token: String) {
        this.baseUrl = baseUrl.trimEnd('/')
        this.token = token
    }

    // ── Data classes ─────────────────────────────────────────────────────────

    data class BrowseAlbum(
        val id: String,
        val title: String,
        val artistName: String?,
        val coverArtKey: String?
    )

    data class BrowseTrack(
        val id: String,
        val title: String,
        val artistName: String?,
        val albumId: String?,
        val albumName: String?,
        val durationMs: Long
    )

    data class BrowsePlaylist(
        val id: String,
        val name: String,
        val description: String?
    )

    data class BrowseArtist(
        val id: String,
        val name: String
    )

    data class AlbumDetail(
        val album: BrowseAlbum,
        val tracks: List<BrowseTrack>
    )

    data class PlaylistDetail(
        val playlist: BrowsePlaylist,
        val tracks: List<BrowseTrack>
    )

    // ── API calls ────────────────────────────────────────────────────────────

    fun recentlyPlayedAlbums(): List<BrowseAlbum> {
        val json = get("/library/recently-played/albums") ?: return emptyList()
        return parseAlbumArray(JSONArray(json))
    }

    fun recentlyAddedAlbums(limit: Int = 20): List<BrowseAlbum> {
        val json = get("/library/recently-added/albums?limit=$limit") ?: return emptyList()
        return parseAlbumArray(JSONArray(json))
    }

    fun albums(limit: Int = 100, offset: Int = 0): List<BrowseAlbum> {
        val json = get("/library/albums?limit=$limit&offset=$offset&sort_by=title") ?: return emptyList()
        val obj = JSONObject(json)
        return parseAlbumArray(obj.optJSONArray("items") ?: return emptyList())
    }

    fun albumDetail(id: String): AlbumDetail? {
        val json = get("/library/albums/$id") ?: return null
        val obj = JSONObject(json)
        val albumObj = obj.optJSONObject("album") ?: return null
        val tracksArr = obj.optJSONArray("tracks") ?: JSONArray()
        return AlbumDetail(
            album = parseAlbum(albumObj),
            tracks = parseTrackArray(tracksArr)
        )
    }

    fun playlists(): List<BrowsePlaylist> {
        val json = get("/playlists") ?: return emptyList()
        val arr = JSONArray(json)
        return (0 until arr.length()).map { i ->
            val o = arr.getJSONObject(i)
            BrowsePlaylist(
                id = o.getString("id"),
                name = o.getString("name"),
                description = optNullableString(o, "description")
            )
        }
    }

    fun playlistDetail(id: String): PlaylistDetail? {
        val json = get("/playlists/$id") ?: return null
        val obj = JSONObject(json)
        val playlistObj = obj.optJSONObject("playlist") ?: return null
        val tracksArr = obj.optJSONArray("tracks") ?: JSONArray()
        return PlaylistDetail(
            playlist = BrowsePlaylist(
                id = playlistObj.getString("id"),
                name = playlistObj.getString("name"),
                description = optNullableString(playlistObj, "description")
            ),
            tracks = parseTrackArray(tracksArr)
        )
    }

    fun favorites(): List<BrowseTrack> {
        val json = get("/library/favorites") ?: return emptyList()
        return parseTrackArray(JSONArray(json))
    }

    fun mostPlayedTracks(limit: Int = 50): List<BrowseTrack> {
        val json = get("/library/most-played?limit=$limit") ?: return emptyList()
        return parseTrackArray(JSONArray(json))
    }

    fun artists(limit: Int = 100, offset: Int = 0): List<BrowseArtist> {
        val json = get("/library/artists?limit=$limit&offset=$offset") ?: return emptyList()
        val obj = JSONObject(json)
        val arr = obj.optJSONArray("items") ?: return emptyList()
        return (0 until arr.length()).map { i ->
            val o = arr.getJSONObject(i)
            BrowseArtist(
                id = o.getString("id"),
                name = o.getString("name")
            )
        }
    }

    fun artistAlbums(artistId: String): List<BrowseAlbum> {
        val json = get("/library/artists/$artistId") ?: return emptyList()
        val obj = JSONObject(json)
        val albumsArr = obj.optJSONArray("albums") ?: return emptyList()
        return parseAlbumArray(albumsArr)
    }

    /**
     * Quick connectivity check — hits /healthz which always returns 200.
     */
    fun isReachable(): Boolean {
        return try {
            val url = URL("$baseUrl/healthz")
            val conn = url.openConnection() as HttpURLConnection
            conn.requestMethod = "GET"
            conn.connectTimeout = 3000
            conn.readTimeout = 3000
            val code = conn.responseCode
            conn.disconnect()
            code == 200
        } catch (_: Exception) {
            false
        }
    }

    /**
     * Fetch autoplay recommendations for the given track.
     * GET /recommend/autoplay?after={trackId}&exclude={ids}&limit={n}
     */
    fun autoplay(afterTrackId: String, excludeIds: List<String> = emptyList(), limit: Int = 10): List<BrowseTrack> {
        val params = StringBuilder("after=$afterTrackId&limit=$limit")
        if (excludeIds.isNotEmpty()) {
            params.append("&exclude=${excludeIds.joinToString(",")}")
        }
        val json = get("/recommend/autoplay?$params") ?: return emptyList()
        return try {
            parseTrackArray(JSONArray(json))
        } catch (e: Exception) {
            Log.w(TAG, "autoplay parse error: ${e.message}")
            emptyList()
        }
    }

    // ── URL builders (for ExoPlayer / cover art) ─────────────────────────────

    fun streamUrl(trackId: String): String =
        "$baseUrl/stream/$trackId?token=$token"

    fun coverUrl(albumId: String): String =
        "$baseUrl/covers/$albumId"

    fun artistCoverUrl(artistId: String): String =
        "$baseUrl/covers/artist/$artistId"

    fun playlistCoverUrl(playlistId: String): String =
        "$baseUrl/covers/playlist/$playlistId/composite"

    // ── HTTP helper ──────────────────────────────────────────────────────────

    private fun get(path: String): String? {
        return try {
            val url = URL("$baseUrl$path")
            val conn = url.openConnection() as HttpURLConnection
            conn.requestMethod = "GET"
            conn.connectTimeout = TIMEOUT
            conn.readTimeout = TIMEOUT
            conn.setRequestProperty("Authorization", "Bearer $token")
            conn.setRequestProperty("Accept", "application/json")

            if (conn.responseCode == 200) {
                conn.inputStream.bufferedReader().use { it.readText() }
            } else {
                Log.w(TAG, "GET $path → ${conn.responseCode}")
                null
            }
        } catch (e: Exception) {
            Log.w(TAG, "GET $path failed: ${e.message}")
            null
        }
    }

    // ── JSON parsing ─────────────────────────────────────────────────────────

    private fun parseAlbumArray(arr: JSONArray): List<BrowseAlbum> {
        return (0 until arr.length()).map { i -> parseAlbum(arr.getJSONObject(i)) }
    }

    private fun parseAlbum(o: JSONObject): BrowseAlbum {
        return BrowseAlbum(
            id = o.getString("id"),
            title = o.getString("title"),
            artistName = optNullableString(o, "artist_name"),
            coverArtKey = optNullableString(o, "cover_art_key")
        )
    }

    private fun parseTrackArray(arr: JSONArray): List<BrowseTrack> {
        return (0 until arr.length()).map { i ->
            val o = arr.getJSONObject(i)
            BrowseTrack(
                id = o.getString("id"),
                title = o.getString("title"),
                artistName = optNullableString(o, "artist_name"),
                albumId = optNullableString(o, "album_id"),
                albumName = optNullableString(o, "album_name"),
                durationMs = o.optLong("duration_ms", 0)
            )
        }
    }

    private fun optNullableString(o: JSONObject, key: String): String? {
        return if (o.isNull(key)) null else o.optString(key, null)
    }
}

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Track {
    pub id: String,
    pub title: String,
    #[serde(default)]
    pub artist: Option<String>,
    #[serde(default)]
    pub album: Option<String>,
    #[serde(default)]
    pub duration_ms: Option<u64>,
    #[serde(default)]
    pub artwork_url: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MediaItem {
    pub id: String,
    pub title: String,
    #[serde(default)]
    pub subtitle: Option<String>,
    #[serde(default)]
    pub playable: bool,
    #[serde(default)]
    pub artwork_url: Option<String>,
    #[serde(default)]
    pub children: Option<Vec<MediaItem>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PlaybackStatePayload {
    pub playing: bool,
    pub position_ms: u64,
}

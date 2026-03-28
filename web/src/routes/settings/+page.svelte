<script lang="ts">
  import { authStore } from '$lib/stores/auth';
  import { validateEmail, validatePassword } from '$lib/utils/validation';
  import { themeStore, avatarStore, ACCENTS } from '$lib/stores/settings/theme';
  import { apiFetch } from '$lib/api/client';
  import { isTauri, isNative } from '$lib/utils/platform';
  import { getApiBase, getServerUrl, setServerUrl } from '$lib/api/base';
  import QRCode from 'qrcode';
  import EQEditor from '$lib/components/ui/EQEditor.svelte';
  import { library } from '$lib/api/library';
  import type { Genre } from '$lib/types';
  import { autoplayEnabled, discordEnabled, replayGainEnabled, smartShuffleEnabled } from '$lib/stores/player';
  import { waveformEnabled, visualizerButtonEnabled, bottomBarSecondary, listenAlongEnabled } from '$lib/stores/settings/theme';
  import { crossfadeEnabled, crossfadeSecs, gaplessEnabled } from '$lib/stores/settings/crossfade';
  import { exclusiveMode, activeDevices, deviceId, deviceName, refreshDevices } from '$lib/stores/player/deviceSession';
  import { devices as devicesApi } from '$lib/api/devices';
  import { downloads, deleteDownload, deleteAllDownloads, getStorageEstimate, retryDownload } from '$lib/stores/offline/downloads';
  import {
    audioOutputDevices,
    selectedAudioOutputId,
    sinkIdSupported,
    refreshAudioOutputDevices,
    setAudioOutput,
    castState,
    castDeviceName,
    initCastSdk,
    startCast,
    stopCast,
  } from '$lib/stores/player/casting';

  // ── Avatar ────────────────────────────────────────────────
  let fileInput: HTMLInputElement;
  let uploadError = '';

  async function handleAvatarUpload(e: Event) {
    uploadError = '';
    const file = (e.target as HTMLInputElement).files?.[0];
    if (!file) return;

    if (!file.type.startsWith('image/')) {
      uploadError = 'Please select an image file.';
      return;
    }

    const img = new Image();
    const blobUrl = URL.createObjectURL(file);
    img.src = blobUrl;
    await new Promise<void>(resolve => { img.onload = () => resolve(); });

    const canvas = document.createElement('canvas');
    canvas.width = 128;
    canvas.height = 128;
    const ctx = canvas.getContext('2d')!;
    const size = Math.min(img.width, img.height);
    const sx = (img.width  - size) / 2;
    const sy = (img.height - size) / 2;
    ctx.drawImage(img, sx, sy, size, size, 0, 0, 128, 128);
    URL.revokeObjectURL(blobUrl);

    avatarStore.set(canvas.toDataURL('image/jpeg', 0.88));
  }

  function clearAvatar() {
    avatarStore.clear();
    if (fileInput) fileInput.value = '';
  }

  // ── Change password ───────────────────────────────────────
  let pwCurrent = '';
  let pwNew = '';
  let pwConfirm = '';
  let pwLoading = false;
  let pwError = '';
  let pwSuccess = false;

  let pwCurrentError = '';
  let pwNewError = '';
  let pwConfirmError = '';

  function blurPwCurrent() { pwCurrentError = pwCurrent ? '' : 'Current password is required.'; }
  function blurPwNew()     { pwNewError = validatePassword(pwNew); }
  function blurPwConfirm() { pwConfirmError = pwConfirm !== pwNew ? 'Passwords do not match.' : (pwConfirm ? '' : 'Please confirm your password.'); }

  async function submitPassword() {
    pwCurrentError = pwCurrent ? '' : 'Current password is required.';
    pwNewError     = validatePassword(pwNew);
    pwConfirmError = !pwConfirm ? 'Please confirm your password.' : pwConfirm !== pwNew ? 'Passwords do not match.' : '';
    pwError = '';
    pwSuccess = false;
    if (pwCurrentError || pwNewError || pwConfirmError) return;
    pwLoading = true;
    try {
      await apiFetch('/auth/password', {
        method: 'PATCH',
        body: JSON.stringify({ current_password: pwCurrent, new_password: pwNew })
      });
      pwCurrent = pwNew = pwConfirm = '';
      pwCurrentError = pwNewError = pwConfirmError = '';
      pwSuccess = true;
    } catch (err: any) {
      pwError = err?.message ?? 'Failed to change password.';
    } finally {
      pwLoading = false;
    }
  }

  // ── Change email ──────────────────────────────────────────
  let emailNew = '';
  let emailPw = '';
  let emailLoading = false;
  let emailError = '';
  let emailSuccess = false;

  let emailNewError = '';
  let emailPwError = '';

  function blurEmailNew() { emailNewError = validateEmail(emailNew); }
  function blurEmailPw()  { emailPwError  = emailPw ? '' : 'Password is required.'; }

  async function submitEmail() {
    emailNewError = validateEmail(emailNew);
    emailPwError  = emailPw ? '' : 'Password is required.';
    emailError = '';
    emailSuccess = false;
    if (emailNewError || emailPwError) return;
    emailLoading = true;
    try {
      await apiFetch('/auth/email', {
        method: 'PATCH',
        body: JSON.stringify({ new_email: emailNew, current_password: emailPw })
      });
      authStore.updateEmail(emailNew);
      emailNew = emailPw = '';
      emailNewError = emailPwError = '';
      emailSuccess = true;
    } catch (err: any) {
      emailError = err?.message ?? 'Failed to change email.';
    } finally {
      emailLoading = false;
    }
  }

  // ── Server URL (native apps only) ──────────────────────
  let serverUrl = getServerUrl();
  let serverSaved = false;

  function saveServer() {
    setServerUrl(serverUrl.replace(/\/+$/, ''));
    serverSaved = true;
    setTimeout(() => serverSaved = false, 2000);
  }

  $: initials = ($authStore.user?.username ?? 'U').slice(0, 2).toUpperCase();

  // ── Two-Factor Authentication ─────────────────────────────
  type TotpStep = 'idle' | 'setup' | 'backup-codes' | 'disable';

  let totpEnabled = false;
  let totpStep: TotpStep = 'idle';
  let totpSecret = '';
  let totpQrUrl = '';
  let totpQrDataUrl = '';
  let totpCode = '';
  let totpDisableCode = '';
  let totpDisablePassword = '';
  let totpBackupCodes: string[] = [];
  let totpLoading = false;
  let totpError = '';
  let totpRegenCode = '';
  let totpRegenLoading = false;
  let totpRegenError = '';
  let totpRegenSuccess = false;
  let totpRegenCodes: string[] = [];

  async function loadTotpStatus() {
    try {
      const res = await apiFetch<{ enabled: boolean }>('/auth/totp/status');
      totpEnabled = res.enabled;
    } catch {}
  }

  loadTotpStatus();

  async function startTotpSetup() {
    totpError = '';
    totpLoading = true;
    try {
      const res = await apiFetch<{ secret: string; otpauth_url: string }>('/auth/totp/setup', { method: 'POST' });
      totpSecret = res.secret;
      totpQrUrl = res.otpauth_url;
      totpQrDataUrl = await QRCode.toDataURL(res.otpauth_url, { width: 200, margin: 1 });
      totpCode = '';
      totpStep = 'setup';
    } catch (err: any) {
      totpError = err?.message ?? 'Failed to start setup.';
    } finally {
      totpLoading = false;
    }
  }

  async function confirmTotpEnable() {
    if (!totpCode) { totpError = 'Enter the 6-digit code from your app.'; return; }
    totpError = '';
    totpLoading = true;
    try {
      const res = await apiFetch<{ backup_codes: string[] }>('/auth/totp/enable', {
        method: 'POST',
        body: JSON.stringify({ code: totpCode })
      });
      totpBackupCodes = res.backup_codes;
      totpEnabled = true;
      totpCode = '';
      totpStep = 'backup-codes';
    } catch (err: any) {
      totpError = err?.message ?? 'Failed to enable 2FA.';
    } finally {
      totpLoading = false;
    }
  }

  async function disableTotp() {
    if (!totpDisablePassword || !totpDisableCode) { totpError = 'Password and code required.'; return; }
    totpError = '';
    totpLoading = true;
    try {
      await apiFetch('/auth/totp/disable', {
        method: 'POST',
        body: JSON.stringify({ password: totpDisablePassword, code: totpDisableCode })
      });
      totpEnabled = false;
      totpDisableCode = '';
      totpDisablePassword = '';
      totpStep = 'idle';
    } catch (err: any) {
      totpError = err?.message ?? 'Failed to disable 2FA.';
    } finally {
      totpLoading = false;
    }
  }

  async function regenBackupCodes() {
    if (!totpRegenCode) { totpRegenError = 'Enter your authenticator code.'; return; }
    totpRegenError = '';
    totpRegenLoading = true;
    totpRegenSuccess = false;
    try {
      const res = await apiFetch<{ backup_codes: string[] }>('/auth/totp/backup-codes/regenerate', {
        method: 'POST',
        body: JSON.stringify({ code: totpRegenCode })
      });
      totpRegenCodes = res.backup_codes;
      totpRegenCode = '';
      totpRegenSuccess = true;
    } catch (err: any) {
      totpRegenError = err?.message ?? 'Failed to regenerate codes.';
    } finally {
      totpRegenLoading = false;
    }
  }

  function closeTotpSetup() {
    totpStep = 'idle';
    totpSecret = '';
    totpQrDataUrl = '';
    totpCode = '';
    totpError = '';
  }

  // ── Email verification ────────────────────────────────────
  let smtpConfigured = false;
  let resendLoading = false;
  let resendSuccess = false;
  let resendError = '';

  async function loadEmailConfig() {
    try {
      const res = await apiFetch<{ verification_enabled: boolean }>('/auth/email-config');
      smtpConfigured = res.verification_enabled;
    } catch {}
  }

  loadEmailConfig();

  async function resendVerification() {
    resendError = '';
    resendSuccess = false;
    resendLoading = true;
    try {
      await apiFetch('/auth/resend-verification', { method: 'POST' });
      resendSuccess = true;
    } catch (err: any) {
      resendError = err?.message ?? 'Failed to send verification email.';
    } finally {
      resendLoading = false;
    }
  }

  // ── Streaming Quality Prefs ────────────────────────────────
  let sqLoading = false;
  let sqSaving  = false;
  let sqError   = '';
  let sqSuccess = false;
  let sqTab: 'any' | 'wifi' | 'mobile' = 'any';
  let sqAnyShowCustom  = false;
  let sqWifiShowCustom = false;
  let sqMobiShowCustom = false;

  let sqMaxBitrate = '';
  let sqMaxSampleRate = '';
  let sqMaxBitDepth = '';
  let sqTranscodeFormat: string | null = null;
  let sqWifiMaxBitrate = '';
  let sqWifiMaxSampleRate = '';
  let sqWifiMaxBitDepth = '';
  let sqWifiTranscodeFormat: string | null = null;
  let sqMobileMaxBitrate = '';
  let sqMobileMaxSampleRate = '';
  let sqMobileMaxBitDepth = '';
  let sqMobileTranscodeFormat: string | null = null;

  const SQ_PRESETS = [
    {
      id: 'unlimited',
      name: 'No Limit',
      desc: 'Original quality',
      detail: 'No restrictions on any parameter',
      wifiDesc: 'Uses your Default quality setting',
      bitrate:    null as number | null,
      sampleRate: null as number | null,
      bitDepth:   null as number | null,
    },
    {
      id: 'hires',
      name: 'Hi-Res',
      desc: 'Studio & hi-res audio',
      detail: 'Up to 192 kHz · 32-bit',
      wifiDesc: 'Up to 192 kHz · 32-bit',
      bitrate:    null as number | null,
      sampleRate: 192000,
      bitDepth:   32,
    },
    {
      id: 'cd',
      name: 'CD Quality',
      desc: 'Standard CD fidelity',
      detail: '44.1 kHz · 16-bit · ~700–1400 kbps',
      wifiDesc: '44.1 kHz · 16-bit · ~700–1400 kbps',
      bitrate:    null as number | null,
      sampleRate: 44100,
      bitDepth:   16,
    },
    {
      id: 'saver',
      name: 'Data Saver',
      desc: 'Cuts bandwidth usage',
      detail: 'Max 320 kbps, CD-quality resolution',
      wifiDesc: 'Max 320 kbps, CD-quality resolution',
      bitrate:    320,
      sampleRate: 44100,
      bitDepth:   16,
    },
  ];

  function formatBytes(bytes: number): string {
    if (bytes >= 1e12) return (bytes / 1e12).toFixed(2) + ' TB';
    if (bytes >= 1e9)  return (bytes / 1e9).toFixed(2) + ' GB';
    if (bytes >= 1e6)  return (bytes / 1e6).toFixed(1) + ' MB';
    if (bytes >= 1e3)  return (bytes / 1e3).toFixed(1) + ' KB';
    return bytes + ' B';
  }

  function parseOptInt(s: string): number | null {
    const n = parseInt(s, 10);
    return s.trim() === '' || isNaN(n) ? null : n;
  }

  function matchPreset(bitrate: string, sampleRate: string, bitDepth: string): string {
    const b = parseOptInt(bitrate);
    const s = parseOptInt(sampleRate);
    const d = parseOptInt(bitDepth);
    for (const p of SQ_PRESETS) {
      if (p.bitrate === b && p.sampleRate === s && p.bitDepth === d) return p.id;
    }
    return 'custom';
  }

  $: anyPresetId    = matchPreset(sqMaxBitrate,       sqMaxSampleRate,       sqMaxBitDepth);
  $: wifiPresetId   = matchPreset(sqWifiMaxBitrate,   sqWifiMaxSampleRate,   sqWifiMaxBitDepth);
  $: mobilePresetId = matchPreset(sqMobileMaxBitrate, sqMobileMaxSampleRate, sqMobileMaxBitDepth);

  $: if (anyPresetId    !== 'custom') sqAnyShowCustom  = false;
  $: if (wifiPresetId   !== 'custom') sqWifiShowCustom = false;
  $: if (mobilePresetId !== 'custom') sqMobiShowCustom = false;

  function applyPreset(tier: 'any' | 'wifi' | 'mobile', p: typeof SQ_PRESETS[number]) {
    const b = p.bitrate    != null ? String(p.bitrate)    : '';
    const s = p.sampleRate != null ? String(p.sampleRate) : '';
    const d = p.bitDepth   != null ? String(p.bitDepth)   : '';
    if (tier === 'any') {
      sqMaxBitrate = b; sqMaxSampleRate = s; sqMaxBitDepth = d;
    } else if (tier === 'wifi') {
      sqWifiMaxBitrate = b; sqWifiMaxSampleRate = s; sqWifiMaxBitDepth = d;
    } else {
      sqMobileMaxBitrate = b; sqMobileMaxSampleRate = s; sqMobileMaxBitDepth = d;
    }
  }

  function toggleCustom(tier: 'any' | 'wifi' | 'mobile') {
    if (tier === 'any')        sqAnyShowCustom  = !sqAnyShowCustom;
    else if (tier === 'wifi')  sqWifiShowCustom = !sqWifiShowCustom;
    else                       sqMobiShowCustom = !sqMobiShowCustom;
  }

  async function loadStreamingPrefs() {
    sqLoading = true;
    try {
      const res = await apiFetch<{
        max_bitrate_kbps:        number | null;
        max_sample_rate:         number | null;
        max_bit_depth:           number | null;
        transcode_format:        string | null;
        wifi_max_bitrate_kbps:   number | null;
        wifi_max_sample_rate:    number | null;
        wifi_max_bit_depth:      number | null;
        wifi_transcode_format:   string | null;
        mobile_max_bitrate_kbps: number | null;
        mobile_max_sample_rate:  number | null;
        mobile_max_bit_depth:    number | null;
        mobile_transcode_format: string | null;
      }>('/user/streaming-prefs');
      sqMaxBitrate            = res.max_bitrate_kbps        != null ? String(res.max_bitrate_kbps)        : '';
      sqMaxSampleRate         = res.max_sample_rate          != null ? String(res.max_sample_rate)          : '';
      sqMaxBitDepth           = res.max_bit_depth            != null ? String(res.max_bit_depth)            : '';
      sqTranscodeFormat       = res.transcode_format         ?? null;
      sqWifiMaxBitrate        = res.wifi_max_bitrate_kbps    != null ? String(res.wifi_max_bitrate_kbps)    : '';
      sqWifiMaxSampleRate     = res.wifi_max_sample_rate     != null ? String(res.wifi_max_sample_rate)     : '';
      sqWifiMaxBitDepth       = res.wifi_max_bit_depth       != null ? String(res.wifi_max_bit_depth)       : '';
      sqWifiTranscodeFormat   = res.wifi_transcode_format    ?? null;
      sqMobileMaxBitrate      = res.mobile_max_bitrate_kbps  != null ? String(res.mobile_max_bitrate_kbps)  : '';
      sqMobileMaxSampleRate   = res.mobile_max_sample_rate   != null ? String(res.mobile_max_sample_rate)   : '';
      sqMobileMaxBitDepth     = res.mobile_max_bit_depth     != null ? String(res.mobile_max_bit_depth)     : '';
      sqMobileTranscodeFormat = res.mobile_transcode_format  ?? null;
    } catch (e) {
      console.error('loadStreamingPrefs error:', e);
    } finally {
      sqLoading = false;
    }
  }

  loadStreamingPrefs();

  async function saveStreamingPrefs() {
    sqError = '';
    sqSuccess = false;
    sqSaving = true;
    try {
      await apiFetch('/user/streaming-prefs', {
        method: 'PUT',
        body: JSON.stringify({
          max_bitrate_kbps:        parseOptInt(sqMaxBitrate),
          max_sample_rate:         parseOptInt(sqMaxSampleRate),
          max_bit_depth:           parseOptInt(sqMaxBitDepth),
          transcode_format:        sqTranscodeFormat,
          wifi_max_bitrate_kbps:   parseOptInt(sqWifiMaxBitrate),
          wifi_max_sample_rate:    parseOptInt(sqWifiMaxSampleRate),
          wifi_max_bit_depth:      parseOptInt(sqWifiMaxBitDepth),
          wifi_transcode_format:   sqWifiTranscodeFormat,
          mobile_max_bitrate_kbps: parseOptInt(sqMobileMaxBitrate),
          mobile_max_sample_rate:  parseOptInt(sqMobileMaxSampleRate),
          mobile_max_bit_depth:    parseOptInt(sqMobileMaxBitDepth),
          mobile_transcode_format: sqMobileTranscodeFormat,
        })
      });
      sqSuccess = true;
      setTimeout(() => sqSuccess = false, 3000);
    } catch (err: any) {
      sqError = err?.message ?? 'Failed to save streaming preferences.';
    } finally {
      sqSaving = false;
    }
  }

  // ── Exclusive Device Mode ──────────────────────────────
  let emSaving = false;
  let emError = '';

  async function toggleExclusiveMode() {
    emError = '';
    emSaving = true;
    try {
      const next = !$exclusiveMode;
      await devicesApi.patchPlaybackSettings({ exclusive_mode: next });
      exclusiveMode.set(next);
    } catch (err: any) {
      emError = err?.message ?? 'Failed to update setting.';
    } finally {
      emSaving = false;
    }
  }

  async function activateDevice(id: string) {
    try {
      await devicesApi.activate(id);
      await refreshDevices();
    } catch { /* ignore */ }
  }

  // ── Audio Output ───────────────────────────────────────
  let audioOutputError = '';

  async function handleAudioOutputChange(e: Event) {
    audioOutputError = '';
    const select = e.target as HTMLSelectElement;
    try {
      await setAudioOutput(select.value);
    } catch (err: any) {
      audioOutputError = err?.message ?? 'Failed to change audio output.';
    }
  }

  // Initialise the Cast SDK so it's ready when the page loads.
  initCastSdk();

  let castError = '';

  async function handleCastClick() {
    castError = '';
    if ($castState === 'connected') {
      stopCast();
    } else {
      try {
        await startCast();
      } catch (err: any) {
        if (err?.code !== 'cancel') {
          castError = 'Could not connect to Cast device. Make sure you are on the same network.';
        }
      }
    }
  }

  // Refresh device list in the background while the page is open.
  let deviceRefreshTimer: ReturnType<typeof setInterval>;
  import { onDestroy } from 'svelte';
  deviceRefreshTimer = setInterval(refreshDevices, 15_000);
  onDestroy(() => clearInterval(deviceRefreshTimer));

  // ── Genres (for EQ per-genre mapping) ─────────────────────
  let allGenres: Genre[] = [];
  library.genres().then(g => { allGenres = g; }).catch(() => {});

  // ── Downloads ─────────────────────────────────────────────
  let storageEst: StorageEstimate | null = null;
  getStorageEstimate().then(e => { storageEst = e; }).catch(() => {});

  let dlSearch = '';
  let expandedAlbums = new Set<string>();

  function toggleAlbumGroup(albumName: string) {
    if (expandedAlbums.has(albumName)) expandedAlbums.delete(albumName);
    else expandedAlbums.add(albumName);
    expandedAlbums = expandedAlbums; // trigger reactivity
  }

  $: allDoneEntries = [...$downloads.values()].filter(d => d.status === 'done');
  $: activeEntries = [...$downloads.values()].filter(d => d.status === 'downloading');
  $: errorEntries = [...$downloads.values()].filter(d => d.status === 'error');

  // Split music vs audiobook
  $: doneEntries = allDoneEntries.filter(e => !e.isAudiobook);
  $: abDoneEntries = allDoneEntries.filter(e => e.isAudiobook);

  $: doneCount = doneEntries.length;
  $: totalSizeBytes = doneEntries.reduce((s, e) => s + e.sizeBytes, 0);
  $: abDoneCount = abDoneEntries.length;
  $: abTotalSizeBytes = abDoneEntries.reduce((s, e) => s + e.sizeBytes, 0);

  $: filteredDone = dlSearch.trim()
    ? doneEntries.filter(e => {
        const q = dlSearch.toLowerCase();
        return e.title.toLowerCase().includes(q) || e.artistName.toLowerCase().includes(q) || e.albumName?.toLowerCase().includes(q);
      })
    : doneEntries;

  // Group music by album
  $: albumGroups = (() => {
    const map = new Map<string, typeof doneEntries>();
    for (const entry of filteredDone) {
      const key = entry.albumName || '';
      const arr = map.get(key);
      if (arr) arr.push(entry);
      else map.set(key, [entry]);
    }
    return [...map.entries()].sort((a, b) => a[0].localeCompare(b[0]));
  })();

  // Group audiobooks by albumId (book id) → book title
  $: audiobookGroups = (() => {
    const map = new Map<string, typeof abDoneEntries>();
    for (const entry of abDoneEntries) {
      const key = entry.albumId || entry.albumName || '';
      const arr = map.get(key);
      if (arr) arr.push(entry);
      else map.set(key, [entry]);
    }
    return [...map.entries()].sort((a, b) => {
      const nameA = a[1][0]?.albumName || '';
      const nameB = b[1][0]?.albumName || '';
      return nameA.localeCompare(nameB);
    });
  })();

  let expandedABGroups = new Set<string>();
  function toggleABGroup(key: string) {
    if (expandedABGroups.has(key)) expandedABGroups.delete(key);
    else expandedABGroups.add(key);
    expandedABGroups = expandedABGroups;
  }

  async function deleteAlbumGroup(tracks: { trackId: string }[]) {
    for (const t of tracks) await deleteDownload(t.trackId);
    storageEst = await getStorageEstimate().catch(() => null);
  }

  let confirmDeleteAll = false;

  async function handleDeleteAll() {
    confirmDeleteAll = false;
    await deleteAllDownloads();
    storageEst = await getStorageEstimate().catch(() => null);
  }

  // ── Version info ──────────────────────────────────────────
  let serverVersion = '';
  let serverSha = '';
  let versionFetchError = '';

  async function fetchServerVersion() {
    try {
      const res = await fetch(`${getApiBase()}/version`);
      if (!res.ok) throw new Error('Failed to fetch server version');
      const data = await res.json();
      serverVersion = data.version || '';
      serverSha = data.sha || '';
      versionFetchError = '';
    } catch (err) {
      versionFetchError = 'Unable to fetch version';
      serverVersion = '';
      serverSha = '';
    }
  }

  import { onMount } from 'svelte';
  import { social } from '$lib/api/social';

  // ── Public profile ────────────────────────────────────────
  let profileDisplayName = '';
  let profileBio = '';
  let profilePublic = false;
  let profileSaving = false;
  let profileSaveMsg = '';
  let profileSaveError = '';

  async function loadPublicProfile() {
    try {
      const p = await social.getMyProfile();
      profileDisplayName = p.display_name ?? '';
      profileBio = p.bio ?? '';
      profilePublic = p.profile_public ?? false;
    } catch {
      // ignore — fields stay blank
    }
  }

  async function savePublicProfile() {
    profileSaving = true;
    profileSaveMsg = '';
    profileSaveError = '';
    try {
      await social.updateMyProfile({
        display_name: profileDisplayName,
        bio: profileBio,
        profile_public: profilePublic,
      });
      profileSaveMsg = 'Profile saved.';
      setTimeout(() => { profileSaveMsg = ''; }, 3000);
    } catch (err: any) {
      profileSaveError = err?.message ?? 'Failed to save profile.';
    } finally {
      profileSaving = false;
    }
  }

  let activeSection = 'profile';

  onMount(() => {
    loadPublicProfile();
    fetchServerVersion();

    // Setup scroll spy for nav active state
    const sections = Array.from(document.querySelectorAll<HTMLElement>('.page > section[id]'));
    if (sections.length === 0) return;

    const scroller = document.querySelector<HTMLElement>('main.content') ?? document.documentElement;

    function updateActiveSection() {
      const scrollTop = scroller.scrollTop;
      const clientHeight = scroller.clientHeight;

      // Activate the last section whose top has crossed 30% down the scroller
      const threshold = scrollTop + clientHeight * 0.3;
      let current = sections[0];
      for (const section of sections) {
        if (section.getBoundingClientRect().top + scrollTop <= threshold) current = section;
      }
      activeSection = current.id;
    }

    scroller.addEventListener('scroll', updateActiveSection, { passive: true });
    updateActiveSection();

    return () => scroller.removeEventListener('scroll', updateActiveSection);
  });
</script>

<div class="settings-shell">
  <nav class="settings-nav">
    <span class="settings-nav-title">Settings</span>
    {#if isNative()}<a class="settings-nav-link" class:active={activeSection === 'server'} href="#server">Server</a>{/if}
    <a class="settings-nav-link" class:active={activeSection === 'profile'} href="#profile">Profile</a>
    <a class="settings-nav-link" class:active={activeSection === 'account'} href="#account">Account</a>
    <a class="settings-nav-link" class:active={activeSection === 'streaming'} href="#streaming">Streaming</a>
    <a class="settings-nav-link" class:active={activeSection === 'eq'} href="#eq">Equalizer</a>
    <a class="settings-nav-link" class:active={activeSection === 'playback'} href="#playback">Playback</a>
    <a class="settings-nav-link" class:active={activeSection === 'devices'} href="#devices">Devices</a>
    <a class="settings-nav-link" class:active={activeSection === 'appearance'} href="#appearance">Appearance</a>
    <a class="settings-nav-link" class:active={activeSection === 'downloads'} href="#downloads">Downloads</a>
    <a class="settings-nav-link" class:active={activeSection === 'about'} href="#about">About</a>
  </nav>

<div class="page">
  <h1 class="page-title">Settings</h1>

  <!-- ── Server (native shells only) ─────────────────────── -->
  {#if isNative()}
  <section id="server" class="card">
    <h2 class="section-title">Server</h2>

    <div class="setting-row setting-row--col" style="border-top:none;padding-top:0">
      <div class="setting-info">
        <span class="setting-name">Server URL</span>
        <span class="setting-desc">The base URL of your Orb server (e.g. https://orb.example.com/api)</span>
      </div>
      <div class="form-grid" style="width:100%">
        <label class="form-label" for="server-url">URL</label>
        <input
          id="server-url"
          class="form-input"
          type="url"
          placeholder="https://orb.example.com/api"
          bind:value={serverUrl}
        />
      </div>
    </div>

    {#if serverSaved}
      <p class="msg msg--ok">Server URL saved.</p>
    {/if}

    <div>
      <button class="btn-primary" on:click={saveServer}>Save</button>
    </div>
  </section>
  {/if}

  <!-- ── Profile ──────────────────────────────────────────── -->
  <section id="profile" class="card">
    <h2 class="section-title">Profile</h2>

    <div class="avatar-area">
      <div class="big-avatar">
        {#if $avatarStore}
          <img src={$avatarStore} alt="" class="avatar-img" />
        {:else}
          <span class="avatar-initials">{initials}</span>
        {/if}
      </div>
      <div class="avatar-actions">
        <button class="btn-primary" on:click={() => fileInput.click()}>
          Upload photo
        </button>
        {#if $avatarStore}
          <button class="btn-ghost" on:click={clearAvatar}>Remove</button>
        {/if}
        {#if uploadError}
          <span class="msg msg--error">{uploadError}</span>
        {/if}
        <input
          bind:this={fileInput}
          type="file"
          accept="image/*"
          style="display:none"
          on:change={handleAvatarUpload}
        />
      </div>
    </div>

    <div class="fields">
      <div class="field">
        <span class="field-label">Username</span>
        <div class="field-value">{$authStore.user?.username ?? '—'}</div>
      </div>
      <div class="field">
        <span class="field-label">Email</span>
        <div class="field-value field-value--row">
          <span>{$authStore.user?.email ?? '—'}</span>
          {#if $authStore.user?.email_verified}
            <span class="badge badge--verified" title="Email verified">Verified</span>
          {:else if smtpConfigured}
            <span class="badge badge--unverified" title="Email not verified">Unverified</span>
          {/if}
        </div>
      </div>
    </div>

    {#if !$authStore.user?.email_verified && smtpConfigured}
      <div class="verify-banner">
        <span class="verify-banner__text">Your email address has not been verified.</span>
        {#if resendSuccess}
          <span class="msg msg--ok" style="margin:0">Verification email sent — check your inbox.</span>
        {:else}
          <button class="btn-ghost btn--sm" on:click={resendVerification} disabled={resendLoading}>
            {resendLoading ? 'Sending…' : 'Resend verification email'}
          </button>
        {/if}
        {#if resendError}<span class="msg msg--error" style="margin:0">{resendError}</span>{/if}
      </div>
    {/if}

    <!-- Public profile fields -->
    <div class="setting-row" style="border-top:none;margin-top:16px;padding-top:0;flex-direction:column;align-items:stretch;gap:12px;">
      <div class="setting-row" style="border-top:none;padding-top:0">
        <div class="setting-info">
          <span class="setting-name">Make profile public</span>
          <span class="setting-desc">Let others find and view your profile, activity, and public playlists.</span>
        </div>
        <button
          class="toggle-btn"
          class:on={profilePublic}
          role="switch"
          aria-checked={profilePublic}
          on:click={() => { profilePublic = !profilePublic; }}
          title={profilePublic ? 'Disable public profile' : 'Enable public profile'}
        ><span class="toggle-knob"></span></button>
      </div>
      <div class="field-col">
        <label class="field-label-sm" for="display-name">Display name</label>
        <input id="display-name" class="form-input" type="text" placeholder={$authStore.user?.username ?? ''} bind:value={profileDisplayName} maxlength="64" />
      </div>
      <div class="field-col">
        <label class="field-label-sm" for="bio">Bio</label>
        <textarea id="bio" class="form-input form-input--textarea" rows="3" placeholder="Tell others about yourself…" bind:value={profileBio} maxlength="300"></textarea>
      </div>
      <div style="display:flex;align-items:center;gap:10px;flex-wrap:wrap;">
        <button class="btn-primary btn--sm" on:click={savePublicProfile} disabled={profileSaving}>
          {profileSaving ? 'Saving…' : 'Save'}
        </button>
        {#if profilePublic && $authStore.user}
          <a href="/profile/{$authStore.user.username}" class="btn-ghost btn--sm">View profile →</a>
        {/if}
        {#if profileSaveMsg}<span class="msg msg--ok">{profileSaveMsg}</span>{/if}
        {#if profileSaveError}<span class="msg msg--error">{profileSaveError}</span>{/if}
      </div>
    </div>
  </section>

  <!-- ── Change password ────────────────────────────────────── -->
  <section id="account" class="card">
    <h2 class="section-title">Change password</h2>

    <div class="form-grid">
      <label class="form-label" for="pw-current">Current password</label>
      <div class="field-col">
        <input
          id="pw-current"
          class="form-input"
          class:form-input--error={!!pwCurrentError}
          type="password"
          autocomplete="current-password"
          bind:value={pwCurrent}
          on:blur={blurPwCurrent}
          disabled={pwLoading}
        />
        {#if pwCurrentError}<span class="field-error">{pwCurrentError}</span>{/if}
      </div>

      <label class="form-label" for="pw-new">New password</label>
      <div class="field-col">
        <input
          id="pw-new"
          class="form-input"
          class:form-input--error={!!pwNewError}
          type="password"
          autocomplete="new-password"
          bind:value={pwNew}
          on:blur={blurPwNew}
          disabled={pwLoading}
        />
        {#if pwNewError}<span class="field-error">{pwNewError}</span>{/if}
      </div>

      <label class="form-label" for="pw-confirm">Confirm new</label>
      <div class="field-col">
        <input
          id="pw-confirm"
          class="form-input"
          class:form-input--error={!!pwConfirmError}
          type="password"
          autocomplete="new-password"
          bind:value={pwConfirm}
          on:blur={blurPwConfirm}
          disabled={pwLoading}
        />
        {#if pwConfirmError}<span class="field-error">{pwConfirmError}</span>{/if}
      </div>
    </div>

    {#if pwError}
      <p class="msg msg--error">{pwError}</p>
    {/if}
    {#if pwSuccess}
      <p class="msg msg--ok">Password updated successfully.</p>
    {/if}

    <div>
      <button class="btn-primary" on:click={submitPassword} disabled={pwLoading}>
        {pwLoading ? 'Saving…' : 'Update password'}
      </button>
    </div>
  </section>

  <!-- ── Change email ──────────────────────────────────────── -->
  <section class="card">
    <h2 class="section-title">Change email</h2>

    <div class="form-grid">
      <label class="form-label" for="email-new">New email</label>
      <div class="field-col">
        <input
          id="email-new"
          class="form-input"
          class:form-input--error={!!emailNewError}
          type="email"
          autocomplete="email"
          bind:value={emailNew}
          on:blur={blurEmailNew}
          disabled={emailLoading}
        />
        {#if emailNewError}<span class="field-error">{emailNewError}</span>{/if}
      </div>

      <label class="form-label" for="email-pw">Current password</label>
      <div class="field-col">
        <input
          id="email-pw"
          class="form-input"
          class:form-input--error={!!emailPwError}
          type="password"
          autocomplete="current-password"
          bind:value={emailPw}
          on:blur={blurEmailPw}
          disabled={emailLoading}
        />
        {#if emailPwError}<span class="field-error">{emailPwError}</span>{/if}
      </div>
    </div>

    {#if emailError}
      <p class="msg msg--error">{emailError}</p>
    {/if}
    {#if emailSuccess}
      <p class="msg msg--ok">Email updated successfully.</p>
    {/if}

    <div>
      <button class="btn-primary" on:click={submitEmail} disabled={emailLoading}>
        {emailLoading ? 'Saving…' : 'Update email'}
      </button>
    </div>
  </section>

  <!-- ── Two-Factor Authentication ────────────────────────── -->
  <section class="card">
    <h2 class="section-title">Two-factor authentication</h2>

    {#if totpStep === 'idle'}
      <div class="setting-row" style="border-top:none;padding-top:0">
        <div class="setting-info">
          <span class="setting-name">Authenticator app</span>
          <span class="setting-desc">
            {totpEnabled ? 'Enabled — your account requires a code on login.' : 'Disabled — add an extra layer of security.'}
          </span>
        </div>
        {#if totpEnabled}
          <button class="btn-danger" on:click={() => { totpStep = 'disable'; totpError = ''; }}>Disable</button>
        {:else}
          <button class="btn-primary" on:click={startTotpSetup} disabled={totpLoading}>
            {totpLoading ? 'Loading…' : 'Set up'}
          </button>
        {/if}
      </div>

      {#if totpEnabled}
        <div class="setting-row">
          <div class="setting-info">
            <span class="setting-name">Backup codes</span>
            <span class="setting-desc">Generate new backup codes (current ones will be invalidated).</span>
          </div>
        </div>
        <div class="form-grid" style="margin-top:-4px">
          <label class="form-label" for="totp-regen-code">Authenticator code</label>
          <input id="totp-regen-code" class="form-input" type="text" inputmode="numeric" maxlength="6"
            placeholder="6-digit code" bind:value={totpRegenCode} disabled={totpRegenLoading} />
        </div>
        {#if totpRegenError}<p class="msg msg--error">{totpRegenError}</p>{/if}
        {#if totpRegenSuccess}
          <p class="msg msg--ok">New backup codes generated — save them now, they won't be shown again.</p>
          <div class="backup-grid">
            {#each totpRegenCodes as code}<code class="backup-code">{code}</code>{/each}
          </div>
        {/if}
        <div>
          <button class="btn-ghost" on:click={regenBackupCodes} disabled={totpRegenLoading}>
            {totpRegenLoading ? 'Generating…' : 'Regenerate backup codes'}
          </button>
        </div>
      {/if}

      {#if totpError}<p class="msg msg--error">{totpError}</p>{/if}

    {:else if totpStep === 'setup'}
      <p class="totp-hint">Scan this QR code with Google Authenticator, Authy, or any TOTP app, then enter the 6-digit code to confirm.</p>
      {#if totpQrDataUrl}
        <div class="qr-wrap"><img src={totpQrDataUrl} alt="TOTP QR code" class="qr-img" /></div>
      {/if}
      <p class="totp-hint" style="margin-top:4px">
        Can't scan? Enter this secret manually:
        <code class="inline-secret">{totpSecret}</code>
      </p>
      <div class="form-grid">
        <label class="form-label" for="totp-confirm-code">Verification code</label>
        <input id="totp-confirm-code" class="form-input totp-input" type="text" inputmode="numeric"
          maxlength="6" placeholder="••••••" bind:value={totpCode} disabled={totpLoading}
          autocomplete="one-time-code" />
      </div>
      {#if totpError}<p class="msg msg--error">{totpError}</p>{/if}
      <div class="btn-row">
        <button class="btn-primary" on:click={confirmTotpEnable} disabled={totpLoading}>
          {totpLoading ? 'Verifying…' : 'Enable 2FA'}
        </button>
        <button class="btn-ghost" on:click={closeTotpSetup} disabled={totpLoading}>Cancel</button>
      </div>

    {:else if totpStep === 'backup-codes'}
      <p class="totp-hint">
        <strong>2FA enabled!</strong> Save these backup codes somewhere safe.
        Each code can be used once to sign in if you lose access to your authenticator app.
        They won't be shown again.
      </p>
      <div class="backup-grid">
        {#each totpBackupCodes as code}<code class="backup-code">{code}</code>{/each}
      </div>
      <div>
        <button class="btn-primary" on:click={closeTotpSetup}>Done</button>
      </div>

    {:else if totpStep === 'disable'}
      <p class="totp-hint">Enter your password and a current authenticator code to disable 2FA.</p>
      <div class="form-grid">
        <label class="form-label" for="totp-dis-pw">Password</label>
        <input id="totp-dis-pw" class="form-input" type="password" autocomplete="current-password"
          bind:value={totpDisablePassword} disabled={totpLoading} />
        <label class="form-label" for="totp-dis-code">Authenticator code</label>
        <input id="totp-dis-code" class="form-input totp-input" type="text" inputmode="numeric"
          maxlength="6" placeholder="••••••" bind:value={totpDisableCode} disabled={totpLoading}
          autocomplete="one-time-code" />
      </div>
      {#if totpError}<p class="msg msg--error">{totpError}</p>{/if}
      <div class="btn-row">
        <button class="btn-danger" on:click={disableTotp} disabled={totpLoading}>
          {totpLoading ? 'Disabling…' : 'Disable 2FA'}
        </button>
        <button class="btn-ghost" on:click={() => { totpStep = 'idle'; totpError = ''; }} disabled={totpLoading}>Cancel</button>
      </div>
    {/if}
  </section>

  <!-- ── Streaming Quality ──────────────────────────────── -->
  <section id="streaming" class="card">
    <h2 class="section-title">Streaming quality</h2>
    <p class="sq-hint">Server-enforced quality limits. Set a different level per connection type to save mobile data while keeping full quality at home.</p>

    {#if sqLoading}
      <p class="msg" style="color:var(--text-2)">Loading…</p>
    {:else}
      <!-- Tab bar -->
      <div class="sq-tabs">
        <button class="sq-tab" class:active={sqTab === 'any'}
          on:click={() => sqTab = 'any'}>
          <svg width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>
            <polyline points="9 22 9 12 15 12 15 22"/>
          </svg>
          Default
        </button>
        <button class="sq-tab" class:active={sqTab === 'wifi'}
          on:click={() => sqTab = 'wifi'}>
          <svg width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <path d="M5 12.55a11 11 0 0 1 14.08 0"/>
            <path d="M1.42 9a16 16 0 0 1 21.16 0"/>
            <path d="M8.53 16.11a6 6 0 0 1 6.95 0"/>
            <circle cx="12" cy="20" r="1" fill="currentColor"/>
          </svg>
          Wi-Fi
        </button>
        <button class="sq-tab" class:active={sqTab === 'mobile'}
          on:click={() => sqTab = 'mobile'}>
          <svg width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <rect x="5" y="2" width="14" height="20" rx="2" ry="2"/>
            <line x1="12" y1="18" x2="12.01" y2="18"/>
          </svg>
          Mobile Data
        </button>
      </div>

      <!-- ── Default tab ── -->
      {#if sqTab === 'any'}
        <div class="sq-presets">
          {#each SQ_PRESETS as p (p.id)}
            <button class="sq-preset" class:active={anyPresetId === p.id}
              on:click={() => applyPreset('any', p)} disabled={sqSaving}>
              <div class="sq-preset-icon">
                {#if p.id === 'unlimited'}
                  <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
                    <path d="M12 12c-2-2.5-4-4-6-4a4 4 0 0 0 0 8c2 0 4-1.5 6-4zm0 0c2 2.5 4 4 6 4a4 4 0 0 0 0-8c-2 0-4 1.5-6 4z"/>
                  </svg>
                {:else if p.id === 'hires'}
                  <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
                    <rect x="2" y="10" width="3" height="10" rx="1" fill="currentColor" stroke="none" opacity="0.5"/>
                    <rect x="8" y="6" width="3" height="14" rx="1" fill="currentColor" stroke="none" opacity="0.7"/>
                    <rect x="14" y="2" width="3" height="18" rx="1" fill="currentColor" stroke="none"/>
                    <rect x="20" y="4" width="2" height="16" rx="1" fill="currentColor" stroke="none"/>
                  </svg>
                {:else if p.id === 'cd'}
                  <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
                    <circle cx="12" cy="12" r="9"/>
                    <circle cx="12" cy="12" r="3"/>
                    <line x1="12" y1="3" x2="12" y2="9"/>
                  </svg>
                {:else if p.id === 'saver'}
                  <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
                    <rect x="5" y="2" width="14" height="20" rx="2"/>
                    <path d="M12 8v6m-3-3 3 3 3-3"/>
                  </svg>
                {/if}
              </div>
              <span class="sq-preset-name">{p.name}</span>
              <span class="sq-preset-desc">{p.desc}</span>
              <span class="sq-preset-detail">{p.detail}</span>
            </button>
          {/each}
        </div>

        <!-- Custom toggle -->
        <button class="sq-custom-btn" class:open={sqAnyShowCustom || anyPresetId === 'custom'}
          on:click={() => toggleCustom('any')} disabled={sqSaving}>
          <svg width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <line x1="4" y1="6" x2="20" y2="6"/><circle cx="8" cy="6" r="2" fill="currentColor" stroke="none"/>
            <line x1="4" y1="12" x2="20" y2="12"/><circle cx="16" cy="12" r="2" fill="currentColor" stroke="none"/>
            <line x1="4" y1="18" x2="20" y2="18"/><circle cx="10" cy="18" r="2" fill="currentColor" stroke="none"/>
          </svg>
          Custom values
          <svg class="sq-chevron" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
            <polyline points="6 9 12 15 18 9"/>
          </svg>
        </button>
        {#if sqAnyShowCustom || anyPresetId === 'custom'}
          <div class="sq-custom-form">
            <div class="sq-custom-row">
              <div class="sq-custom-info">
                <span class="sq-custom-label">Max bitrate <code>kbps</code></span>
                <span class="sq-custom-sub">Controls transfer speed. FLAC at CD quality ≈ 700–1400 kbps.</span>
              </div>
              <input class="form-input sq-custom-input" type="number" min="1" placeholder="no limit"
                bind:value={sqMaxBitrate} disabled={sqSaving} />
            </div>
            <div class="sq-custom-row">
              <div class="sq-custom-info">
                <span class="sq-custom-label">Max sample rate <code>Hz</code></span>
                <span class="sq-custom-sub">Frequency ceiling. 44100 = CD, 96000 = hi-res, 192000 = studio.</span>
              </div>
              <input class="form-input sq-custom-input" type="number" min="1" placeholder="no limit"
                bind:value={sqMaxSampleRate} disabled={sqSaving} />
            </div>
            <div class="sq-custom-row">
              <div class="sq-custom-info">
                <span class="sq-custom-label">Max bit depth <code>bits</code></span>
                <span class="sq-custom-sub">Dynamic range. 16 = CD standard, 24 or 32 = hi-res.</span>
              </div>
              <input class="form-input sq-custom-input" type="number" min="1" placeholder="no limit"
                bind:value={sqMaxBitDepth} disabled={sqSaving} />
            </div>
          </div>
        {/if}

        <!-- Transcode format -->
        <div class="sq-transcode-row">
          <div class="sq-custom-info">
            <span class="sq-custom-label">Transcode format</span>
            <span class="sq-custom-sub">Re-encode on the server before delivery. Enables real bitrate reduction. Requires ffmpeg. Seeking may be limited.</span>
          </div>
          <select class="form-input sq-transcode-select" bind:value={sqTranscodeFormat} disabled={sqSaving}>
            <option value={null}>Off — pass-through</option>
            <option value="mp3">MP3</option>
            <option value="aac">AAC</option>
            <option value="opus">Opus (Ogg)</option>
          </select>
        </div>

      <!-- ── Wi-Fi tab ── -->
      {:else if sqTab === 'wifi'}
        <p class="sq-tab-note">Overrides Default when your device is on Wi-Fi. Leave at "No Limit" to simply use your Default setting.</p>
        <div class="sq-presets">
          {#each SQ_PRESETS as p (p.id)}
            <button class="sq-preset" class:active={wifiPresetId === p.id}
              on:click={() => applyPreset('wifi', p)} disabled={sqSaving}>
              <div class="sq-preset-icon">
                {#if p.id === 'unlimited'}
                  <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
                    <path d="M12 12c-2-2.5-4-4-6-4a4 4 0 0 0 0 8c2 0 4-1.5 6-4zm0 0c2 2.5 4 4 6 4a4 4 0 0 0 0-8c-2 0-4 1.5-6 4z"/>
                  </svg>
                {:else if p.id === 'hires'}
                  <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
                    <rect x="2" y="10" width="3" height="10" rx="1" fill="currentColor" stroke="none" opacity="0.5"/>
                    <rect x="8" y="6" width="3" height="14" rx="1" fill="currentColor" stroke="none" opacity="0.7"/>
                    <rect x="14" y="2" width="3" height="18" rx="1" fill="currentColor" stroke="none"/>
                    <rect x="20" y="4" width="2" height="16" rx="1" fill="currentColor" stroke="none"/>
                  </svg>
                {:else if p.id === 'cd'}
                  <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
                    <circle cx="12" cy="12" r="9"/>
                    <circle cx="12" cy="12" r="3"/>
                    <line x1="12" y1="3" x2="12" y2="9"/>
                  </svg>
                {:else if p.id === 'saver'}
                  <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
                    <rect x="5" y="2" width="14" height="20" rx="2"/>
                    <path d="M12 8v6m-3-3 3 3 3-3"/>
                  </svg>
                {/if}
              </div>
              <span class="sq-preset-name">{p.id === 'unlimited' ? 'Use Default' : p.name}</span>
              <span class="sq-preset-desc">{p.id === 'unlimited' ? 'Follow Default setting' : p.desc}</span>
              <span class="sq-preset-detail">{p.id === 'unlimited' ? 'No Wi-Fi override applied' : p.wifiDesc}</span>
            </button>
          {/each}
        </div>

        <button class="sq-custom-btn" class:open={sqWifiShowCustom || wifiPresetId === 'custom'}
          on:click={() => toggleCustom('wifi')} disabled={sqSaving}>
          <svg width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <line x1="4" y1="6" x2="20" y2="6"/><circle cx="8" cy="6" r="2" fill="currentColor" stroke="none"/>
            <line x1="4" y1="12" x2="20" y2="12"/><circle cx="16" cy="12" r="2" fill="currentColor" stroke="none"/>
            <line x1="4" y1="18" x2="20" y2="18"/><circle cx="10" cy="18" r="2" fill="currentColor" stroke="none"/>
          </svg>
          Custom values
          <svg class="sq-chevron" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
            <polyline points="6 9 12 15 18 9"/>
          </svg>
        </button>
        {#if sqWifiShowCustom || wifiPresetId === 'custom'}
          <div class="sq-custom-form">
            <div class="sq-custom-row">
              <div class="sq-custom-info">
                <span class="sq-custom-label">Max bitrate <code>kbps</code></span>
                <span class="sq-custom-sub">Leave blank to inherit from Default.</span>
              </div>
              <input class="form-input sq-custom-input" type="number" min="1" placeholder="inherit default"
                bind:value={sqWifiMaxBitrate} disabled={sqSaving} />
            </div>
            <div class="sq-custom-row">
              <div class="sq-custom-info">
                <span class="sq-custom-label">Max sample rate <code>Hz</code></span>
                <span class="sq-custom-sub">Leave blank to inherit from Default.</span>
              </div>
              <input class="form-input sq-custom-input" type="number" min="1" placeholder="inherit default"
                bind:value={sqWifiMaxSampleRate} disabled={sqSaving} />
            </div>
            <div class="sq-custom-row">
              <div class="sq-custom-info">
                <span class="sq-custom-label">Max bit depth <code>bits</code></span>
                <span class="sq-custom-sub">Leave blank to inherit from Default.</span>
              </div>
              <input class="form-input sq-custom-input" type="number" min="1" placeholder="inherit default"
                bind:value={sqWifiMaxBitDepth} disabled={sqSaving} />
            </div>
          </div>
        {/if}

        <!-- Transcode format -->
        <div class="sq-transcode-row">
          <div class="sq-custom-info">
            <span class="sq-custom-label">Transcode format</span>
            <span class="sq-custom-sub">Override transcode format on Wi-Fi. Null inherits from Default.</span>
          </div>
          <select class="form-input sq-transcode-select" bind:value={sqWifiTranscodeFormat} disabled={sqSaving}>
            <option value={null}>Inherit default</option>
            <option value="mp3">MP3</option>
            <option value="aac">AAC</option>
            <option value="opus">Opus (Ogg)</option>
          </select>
        </div>

      <!-- ── Mobile Data tab ── -->
      {:else if sqTab === 'mobile'}
        <p class="sq-tab-note">Overrides Default on cellular connections. Set "Data Saver" here to protect your mobile data plan.</p>
        <div class="sq-presets">
          {#each SQ_PRESETS as p (p.id)}
            <button class="sq-preset" class:active={mobilePresetId === p.id}
              on:click={() => applyPreset('mobile', p)} disabled={sqSaving}>
              <div class="sq-preset-icon">
                {#if p.id === 'unlimited'}
                  <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
                    <path d="M12 12c-2-2.5-4-4-6-4a4 4 0 0 0 0 8c2 0 4-1.5 6-4zm0 0c2 2.5 4 4 6 4a4 4 0 0 0 0-8c-2 0-4 1.5-6 4z"/>
                  </svg>
                {:else if p.id === 'hires'}
                  <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
                    <rect x="2" y="10" width="3" height="10" rx="1" fill="currentColor" stroke="none" opacity="0.5"/>
                    <rect x="8" y="6" width="3" height="14" rx="1" fill="currentColor" stroke="none" opacity="0.7"/>
                    <rect x="14" y="2" width="3" height="18" rx="1" fill="currentColor" stroke="none"/>
                    <rect x="20" y="4" width="2" height="16" rx="1" fill="currentColor" stroke="none"/>
                  </svg>
                {:else if p.id === 'cd'}
                  <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
                    <circle cx="12" cy="12" r="9"/>
                    <circle cx="12" cy="12" r="3"/>
                    <line x1="12" y1="3" x2="12" y2="9"/>
                  </svg>
                {:else if p.id === 'saver'}
                  <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
                    <rect x="5" y="2" width="14" height="20" rx="2"/>
                    <path d="M12 8v6m-3-3 3 3 3-3"/>
                  </svg>
                {/if}
              </div>
              <span class="sq-preset-name">{p.id === 'unlimited' ? 'Use Default' : p.name}</span>
              <span class="sq-preset-desc">{p.id === 'unlimited' ? 'Follow Default setting' : p.desc}</span>
              <span class="sq-preset-detail">{p.id === 'unlimited' ? 'No mobile override applied' : p.wifiDesc}</span>
              {#if p.id === 'saver'}
                <span class="sq-preset-badge">Recommended</span>
              {/if}
            </button>
          {/each}
        </div>

        <button class="sq-custom-btn" class:open={sqMobiShowCustom || mobilePresetId === 'custom'}
          on:click={() => toggleCustom('mobile')} disabled={sqSaving}>
          <svg width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <line x1="4" y1="6" x2="20" y2="6"/><circle cx="8" cy="6" r="2" fill="currentColor" stroke="none"/>
            <line x1="4" y1="12" x2="20" y2="12"/><circle cx="16" cy="12" r="2" fill="currentColor" stroke="none"/>
            <line x1="4" y1="18" x2="20" y2="18"/><circle cx="10" cy="18" r="2" fill="currentColor" stroke="none"/>
          </svg>
          Custom values
          <svg class="sq-chevron" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
            <polyline points="6 9 12 15 18 9"/>
          </svg>
        </button>
        {#if sqMobiShowCustom || mobilePresetId === 'custom'}
          <div class="sq-custom-form">
            <div class="sq-custom-row">
              <div class="sq-custom-info">
                <span class="sq-custom-label">Max bitrate <code>kbps</code></span>
                <span class="sq-custom-sub">Leave blank to inherit from Default.</span>
              </div>
              <input class="form-input sq-custom-input" type="number" min="1" placeholder="inherit default"
                bind:value={sqMobileMaxBitrate} disabled={sqSaving} />
            </div>
            <div class="sq-custom-row">
              <div class="sq-custom-info">
                <span class="sq-custom-label">Max sample rate <code>Hz</code></span>
                <span class="sq-custom-sub">Leave blank to inherit from Default.</span>
              </div>
              <input class="form-input sq-custom-input" type="number" min="1" placeholder="inherit default"
                bind:value={sqMobileMaxSampleRate} disabled={sqSaving} />
            </div>
            <div class="sq-custom-row">
              <div class="sq-custom-info">
                <span class="sq-custom-label">Max bit depth <code>bits</code></span>
                <span class="sq-custom-sub">Leave blank to inherit from Default.</span>
              </div>
              <input class="form-input sq-custom-input" type="number" min="1" placeholder="inherit default"
                bind:value={sqMobileMaxBitDepth} disabled={sqSaving} />
            </div>
          </div>
        {/if}

        <!-- Transcode format -->
        <div class="sq-transcode-row">
          <div class="sq-custom-info">
            <span class="sq-custom-label">Transcode format</span>
            <span class="sq-custom-sub">Override transcode format on mobile data. Null inherits from Default.</span>
          </div>
          <select class="form-input sq-transcode-select" bind:value={sqMobileTranscodeFormat} disabled={sqSaving}>
            <option value={null}>Inherit default</option>
            <option value="mp3">MP3</option>
            <option value="aac">AAC</option>
            <option value="opus">Opus (Ogg)</option>
          </select>
        </div>
      {/if}

      {#if sqError}<p class="msg msg--error">{sqError}</p>{/if}
      {#if sqSuccess}<p class="msg msg--ok">Streaming preferences saved.</p>{/if}

      <div>
        <button class="btn-primary" on:click={saveStreamingPrefs} disabled={sqSaving}>
          {sqSaving ? 'Saving…' : 'Save'}
        </button>
      </div>
    {/if}
  </section>

  <!-- ── Equalizer ──────────────────────────────────────── -->
  <section id="eq" class="card">
    <h2 class="section-title">Equalizer</h2>
    <EQEditor genres={allGenres} />
  </section>

  <!-- ── Playback ───────────────────────────────────────── -->
  <section id="playback" class="card">
    <h2 class="section-title">Playback</h2>

    <div class="setting-row" style="border-top:none;padding-top:0">
      <div class="setting-info">
        <span class="setting-name">Autoplay</span>
        <span class="setting-desc">
          When the queue ends, automatically add similar tracks based on what you were listening to.
        </span>
      </div>
      <button
        class="toggle-btn"
        class:on={$autoplayEnabled}
        role="switch"
        aria-checked={$autoplayEnabled}
        on:click={() => autoplayEnabled.set(!$autoplayEnabled)}
        title={$autoplayEnabled ? 'Disable autoplay' : 'Enable autoplay'}
      >
        <span class="toggle-knob"></span>
      </button>
      </div>

      <div class="setting-row">
      <div class="setting-info">
        <span class="setting-name">Visualizer button</span>
        <span class="setting-desc">
          Show the sound visualizer toggle in the bottom player bar.
        </span>
      </div>
      <button
        class="toggle-btn"
        class:on={$visualizerButtonEnabled}
        role="switch"
        aria-checked={$visualizerButtonEnabled}
        on:click={() => visualizerButtonEnabled.toggle()}
        title={$visualizerButtonEnabled ? 'Disable visualizer button' : 'Enable visualizer button'}
      >
        <span class="toggle-knob"></span>
      </button>
      </div>

      <div class="setting-row">
      <div class="setting-info">
        <span class="setting-name">Listen Along button</span>
        <span class="setting-desc">
          Show the Listen Along button in the bottom player bar.
        </span>
      </div>
      <button
        class="toggle-btn"
        class:on={$listenAlongEnabled}
        role="switch"
        aria-checked={$listenAlongEnabled}
        on:click={() => listenAlongEnabled.toggle()}
        title={$listenAlongEnabled ? 'Disable Listen Along button' : 'Enable Listen Along button'}
      >
        <span class="toggle-knob"></span>
      </button>
      </div>

      <div class="setting-row">
      <div class="setting-info">
        <span class="setting-name">Waveform seek bar</span>        <span class="setting-desc">
          Show a waveform visualisation instead of a plain seek bar. Pre-generated during ingest using audiowaveform; falls back to client-side computation.
        </span>
      </div>
      <button
        class="toggle-btn"
        class:on={$waveformEnabled}
        role="switch"
        aria-checked={$waveformEnabled}
        on:click={() => waveformEnabled.set(!$waveformEnabled)}
        title={$waveformEnabled ? 'Disable waveform seek bar' : 'Enable waveform seek bar'}
      >
        <span class="toggle-knob"></span>
      </button>
    </div>

    <div class="setting-row">
      <div class="setting-info">
        <span class="setting-name">Smart Shuffle</span>
        <span class="setting-desc">
          When shuffle is on, spread tracks by artist so the same artist never plays back-to-back,
          and move recently played tracks toward the end of the order.
        </span>
      </div>
      <button
        class="toggle-btn"
        class:on={$smartShuffleEnabled}
        role="switch"
        aria-checked={$smartShuffleEnabled}
        on:click={() => smartShuffleEnabled.set(!$smartShuffleEnabled)}
        title={$smartShuffleEnabled ? 'Disable smart shuffle' : 'Enable smart shuffle'}
      >
        <span class="toggle-knob"></span>
      </button>
    </div>

    <div class="setting-row">
      <div class="setting-info">
        <span class="setting-name">Auto Volume Leveling</span>
        <span class="setting-desc">
          Normalize track loudness using ReplayGain metadata so every track plays at a consistent volume.
        </span>
      </div>
      <button
        class="toggle-btn"
        class:on={$replayGainEnabled}
        role="switch"
        aria-checked={$replayGainEnabled}
        on:click={() => replayGainEnabled.set(!$replayGainEnabled)}
        title={$replayGainEnabled ? 'Disable auto volume leveling' : 'Enable auto volume leveling'}
      >
        <span class="toggle-knob"></span>
      </button>
    </div>

    <div class="setting-row">
      <div class="setting-info">
        <span class="setting-name">Gapless Playback</span>
        <span class="setting-desc">
          Schedule the next track to start the instant the current one ends — no silence between tracks.
          Only applies to 24-bit Hi-Res tracks on the Web Audio path.
          Disabled when Crossfade is on.
        </span>
      </div>
      <button
        class="toggle-btn"
        class:on={$gaplessEnabled && !$crossfadeEnabled}
        role="switch"
        aria-checked={$gaplessEnabled && !$crossfadeEnabled}
        disabled={$crossfadeEnabled}
        on:click={() => gaplessEnabled.set(!$gaplessEnabled)}
        title={$gaplessEnabled ? 'Disable gapless playback' : 'Enable gapless playback'}
      >
        <span class="toggle-knob"></span>
      </button>
    </div>

    <div class="setting-row">
      <div class="setting-info">
        <span class="setting-name">Crossfade</span>
        <span class="setting-desc">
          Blend the end of one track into the start of the next using overlapping volume fades.
          Only applies to 24-bit Hi-Res tracks on the Web Audio path.
          When on, overrides Gapless Playback.
        </span>
      </div>
      <button
        class="toggle-btn"
        class:on={$crossfadeEnabled}
        role="switch"
        aria-checked={$crossfadeEnabled}
        on:click={() => crossfadeEnabled.set(!$crossfadeEnabled)}
        title={$crossfadeEnabled ? 'Disable crossfade' : 'Enable crossfade'}
      >
        <span class="toggle-knob"></span>
      </button>
    </div>

    {#if $crossfadeEnabled}
    <div class="setting-row" style="align-items:center; gap:1rem;">
      <div class="setting-info" style="flex:1 1 auto;">
        <span class="setting-name">Crossfade Duration</span>
        <span class="setting-desc">How many seconds the outgoing and incoming tracks overlap.</span>
      </div>
      <div style="display:flex; align-items:center; gap:0.6rem; flex-shrink:0;">
        <input
          type="range"
          min="1"
          max="12"
          step="0.5"
          value={$crossfadeSecs}
          on:input={(e) => crossfadeSecs.set(parseFloat((e.target as HTMLInputElement).value))}
          style="width:120px; accent-color:var(--accent);"
        />
        <span style="min-width:2.5rem; text-align:right; font-size:0.85rem; color:var(--text-secondary);">{$crossfadeSecs}s</span>
      </div>
    </div>
    {/if}

    {#if isTauri()}
    <div class="setting-row">
      <div class="setting-info">
        <span class="setting-name">Discord Rich Presence</span>
        <span class="setting-desc">
          Show what you're listening to in your Discord status. Requires Discord to be running.
        </span>
      </div>
      <button
        class="toggle-btn"
        class:on={$discordEnabled}
        role="switch"
        aria-checked={$discordEnabled}
        on:click={() => discordEnabled.set(!$discordEnabled)}
        title={$discordEnabled ? 'Disable Discord presence' : 'Enable Discord presence'}
      >
        <span class="toggle-knob"></span>
      </button>
    </div>
    {/if}
  </section>

  <!-- ── Devices ────────────────────────────────────────── -->
  <section id="devices" class="card">
    <h2 class="section-title">Devices</h2>

    <!-- Exclusive mode toggle -->
    <div class="setting-row" style="border-top:none;padding-top:0">
      <div class="setting-info">
        <span class="setting-name">Exclusive Device Mode</span>
        <span class="setting-desc">
          When enabled, only one device can play at a time. Switching devices pauses all others. Disable to stream different tracks from multiple devices simultaneously.
        </span>
      </div>
      <button
        class="toggle-btn"
        class:on={$exclusiveMode}
        role="switch"
        aria-checked={$exclusiveMode}
        on:click={toggleExclusiveMode}
        disabled={emSaving}
        title={$exclusiveMode ? 'Disable exclusive mode' : 'Enable exclusive mode'}
      >
        <span class="toggle-knob"></span>
      </button>
    </div>
    {#if emError}<p class="msg msg--error">{emError}</p>{/if}

    <!-- Audio Output selection -->
    {#if sinkIdSupported}
      <div class="setting-row setting-row--col" style="margin-top:4px">
        <div class="setting-info">
          <span class="setting-name">Audio output</span>
          <span class="setting-desc">
            Route music to a different audio output — Bluetooth speakers, HDMI, USB DAC, etc.
            Only outputs paired to this computer appear here.
          </span>
        </div>
        <div style="display:flex;align-items:center;gap:8px;margin-top:8px;flex-wrap:wrap">
          <select
            class="form-input"
            style="width:auto;min-width:200px;max-width:360px"
            value={$selectedAudioOutputId}
            on:change={handleAudioOutputChange}
            on:focus={refreshAudioOutputDevices}
          >
            {#if $audioOutputDevices.length === 0}
              <option value="default">System Default</option>
            {:else}
              {#each $audioOutputDevices as dev (dev.deviceId)}
                <option value={dev.deviceId}>{dev.label}</option>
              {/each}
            {/if}
          </select>
          <button
            class="btn-ghost"
            style="font-size:11px;padding:5px 10px"
            on:click={refreshAudioOutputDevices}
            title="Refresh audio output list"
          >Refresh</button>
        </div>
        {#if audioOutputError}<p class="msg msg--error">{audioOutputError}</p>{/if}
      </div>
    {/if}

    <!-- Chromecast -->
    {#if $castState !== 'unavailable'}
      <div class="setting-row" style="margin-top:4px">
        <div class="setting-info">
          <span class="setting-name">Cast to device</span>
          <span class="setting-desc">
            {#if $castState === 'connected'}
              Casting to <strong>{$castDeviceName}</strong>. Music plays on the remote device; stop casting to return to local playback.
            {:else}
              Stream music to a Chromecast, smart TV, or any Cast-enabled speaker on your network.
            {/if}
          </span>
        </div>
        <button
          class="btn-{$castState === 'connected' ? 'danger' : 'primary'}"
          style="white-space:nowrap;min-width:120px"
          on:click={handleCastClick}
          disabled={$castState === 'connecting'}
        >
          {#if $castState === 'connecting'}
            Connecting…
          {:else if $castState === 'connected'}
            Stop casting
          {:else}
            Cast audio
          {/if}
        </button>
      </div>
      {#if castError}<p class="msg msg--error">{castError}</p>{/if}
    {/if}

    <!-- Active device list -->
    <div class="setting-row setting-row--col" style="margin-top:4px">
      <div class="setting-info">
        <span class="setting-name">Active sessions</span>
        <span class="setting-desc">All devices currently registered under your account. Sessions expire after 90 s of inactivity.</span>
      </div>

      {#if $activeDevices.length === 0}
        <p style="font-size:12px;color:var(--text-2);margin-top:8px">No active sessions found.</p>
      {:else}
        <div class="device-list">
          {#each $activeDevices as dev (dev.id)}
            <div class="device-card" class:device-card--active={dev.is_active}>
              <div class="device-card-icon">
                <svg width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24">
                  <rect x="2" y="3" width="20" height="14" rx="2"/>
                  <path d="M8 21h8m-4-4v4"/>
                </svg>
              </div>
              <div class="device-card-info">
                <span class="device-card-name">
                  {dev.name}
                  {#if dev.id === deviceId}<span class="device-card-badge">This device</span>{/if}
                  {#if dev.is_active}<span class="device-card-badge device-card-badge--active">Active</span>{/if}
                </span>
                {#if dev.state?.track_title}
                  <span class="device-card-track">
                    {#if dev.state.playing}
                      <svg width="10" height="10" fill="currentColor" viewBox="0 0 24 24" style="color:var(--accent)"><path d="M8 5v14l11-7z"/></svg>
                    {:else}
                      <svg width="10" height="10" fill="currentColor" viewBox="0 0 24 24" style="color:var(--text-2)"><path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/></svg>
                    {/if}
                    {dev.state.track_title}
                  </span>
                {:else}
                  <span class="device-card-track" style="color:var(--text-2)">Idle</span>
                {/if}
              </div>
              {#if $exclusiveMode && dev.id !== deviceId}
                <button
                  class="btn-ghost"
                  style="font-size:11px;padding:5px 10px;white-space:nowrap"
                  on:click={() => activateDevice(dev.id)}
                  title="Transfer playback to this device"
                >
                  Transfer here
                </button>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </section>

  <!-- ── Appearance ─────────────────────────────────────── -->
  <section id="appearance" class="card">
    <h2 class="section-title">Appearance</h2>

    <div class="setting-row">
      <div class="setting-info">
        <span class="setting-name">Color mode</span>
        <span class="setting-desc">Choose between dark and light interface</span>
      </div>
      <div class="mode-toggle">
        <button
          class="mode-btn"
          class:active={$themeStore.mode === 'dark'}
          on:click={() => themeStore.setMode('dark')}
        >
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
          </svg>
          Dark
        </button>
        <button
          class="mode-btn"
          class:active={$themeStore.mode === 'light'}
          on:click={() => themeStore.setMode('light')}
        >
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <circle cx="12" cy="12" r="5"/>
            <line x1="12" y1="1"  x2="12" y2="3"/>
            <line x1="12" y1="21" x2="12" y2="23"/>
            <line x1="4.22" y1="4.22"  x2="5.64" y2="5.64"/>
            <line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/>
            <line x1="1" y1="12" x2="3" y2="12"/>
            <line x1="21" y1="12" x2="23" y2="12"/>
            <line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/>
            <line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/>
          </svg>
          Light
        </button>
      </div>
    </div>

    <div class="setting-row setting-row--col">
      <div class="setting-info">
        <span class="setting-name">Accent color</span>
        <span class="setting-desc">Used for highlights, active states, and interactive elements</span>
      </div>
      <div class="palette">
        {#each ACCENTS as accent}
          <button
            class="swatch"
            class:selected={$themeStore.accent === accent.value}
            style="--swatch: {accent.value}; --swatch-glow: rgba({accent.rgb},0.4);"
            title={accent.name}
            on:click={() => themeStore.setAccent(accent.value)}
            aria-label={accent.name}
            aria-pressed={$themeStore.accent === accent.value}
          >
            {#if $themeStore.accent === accent.value}
              <svg width="10" height="10" fill="none" stroke="white" stroke-width="2.5" viewBox="0 0 24 24">
                <polyline points="20 6 9 17 4 12"/>
              </svg>
            {/if}
          </button>
        {/each}
      </div>
    </div>

    <div class="setting-row">
      <div class="setting-info">
        <span class="setting-name">Bottom bar secondary info</span>
        <span class="setting-desc">What to show below the track title in the player bar</span>
      </div>
      <div class="mode-toggle">
        <button
          class="mode-btn"
          class:active={$bottomBarSecondary === 'album'}
          on:click={() => bottomBarSecondary.set('album')}
        >
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <rect x="3" y="3" width="18" height="18" rx="2"/>
            <circle cx="12" cy="12" r="4"/>
            <circle cx="12" cy="12" r="1" fill="currentColor"/>
          </svg>
          Album
        </button>
        <button
          class="mode-btn"
          class:active={$bottomBarSecondary === 'artist'}
          on:click={() => bottomBarSecondary.set('artist')}
        >
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <circle cx="12" cy="8" r="4"/>
            <path d="M4 20c0-4 3.6-7 8-7s8 3 8 7"/>
          </svg>
          Artist
        </button>
      </div>
    </div>
  </section>

  <!-- ── Downloads ─────────────────────────────────────── -->
  <section id="downloads" class="card">
    <h2 class="section-title">Downloads</h2>

    <!-- Storage summary -->
    <div class="dl-summary">
      <span class="dl-stat">{doneCount + abDoneCount} file{doneCount + abDoneCount !== 1 ? 's' : ''} · {formatBytes(totalSizeBytes + abTotalSizeBytes)}</span>
      {#if storageEst}
        <span class="dl-stat dl-stat--muted">
          {formatBytes(storageEst.usage ?? 0)} used{#if storageEst.quota} of {formatBytes(storageEst.quota)}{/if}
        </span>
      {/if}
    </div>

    <!-- Active downloads -->
    {#if activeEntries.length > 0}
      <div class="dl-active">
        <h3 class="dl-subhead">Downloading ({activeEntries.length})</h3>
        {#each activeEntries as entry (entry.trackId)}
          <div class="dl-active-item">
            <div class="dl-active-info">
              <span class="dl-active-title">{entry.title}</span>
              <span class="dl-active-meta">{entry.albumName || entry.artistName || ''} · {entry.progress}%</span>
            </div>
            <div class="dl-progress-bar">
              <div class="dl-progress-fill" style="width:{entry.progress}%"></div>
            </div>
          </div>
        {/each}
      </div>
    {/if}

    <!-- Errored downloads -->
    {#if errorEntries.length > 0}
      <div class="dl-errors">
        <h3 class="dl-subhead dl-subhead--warn">Failed ({errorEntries.length})</h3>
        {#each errorEntries as entry (entry.trackId)}
          <div class="dl-error-item">
            <span class="dl-error-title">{entry.title}</span>
            {#if entry.artistName}<span class="dl-error-artist">{entry.artistName}</span>{/if}
            <span class="dl-error-msg">{entry.error || 'Unknown error'}</span>
            <button class="btn-ghost" style="font-size:11px;padding:2px 8px" on:click={() => retryDownload(entry)}>Retry</button>
            <button class="btn-ghost" style="font-size:11px;padding:2px 8px" on:click={() => deleteDownload(entry.trackId)}>Dismiss</button>
          </div>
        {/each}
      </div>
    {/if}

    <!-- ── Music ─────────────────── -->
    {#if doneCount > 0}
      <div class="dl-section-head">
        <h3 class="dl-subhead">Music</h3>
        <span class="dl-section-stat">{doneCount} track{doneCount !== 1 ? 's' : ''} · {formatBytes(totalSizeBytes)}</span>
      </div>
      {#if doneCount > 10}
        <input class="dl-search" type="text" placeholder="Search music…" bind:value={dlSearch} />
      {/if}

      {#each albumGroups as [albumName, tracks] (albumName)}
        <div class="dl-album-group">
          <div class="dl-album-header-row">
            <button class="dl-album-header" on:click={() => toggleAlbumGroup(albumName)}>
              <svg class="dl-chevron" class:expanded={expandedAlbums.has(albumName)} width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>
              <span class="dl-album-name">{albumName || 'Unknown Album'}</span>
              <span class="dl-album-count">{tracks.length} · {formatBytes(tracks.reduce((s, e) => s + e.sizeBytes, 0))}</span>
            </button>
            <button class="dl-remove-btn dl-remove-btn--album" on:click={() => deleteAlbumGroup(tracks)} title="Delete album downloads">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14H6L5 6"/><path d="M10 11v6M14 11v6"/><path d="M9 6V4h6v2"/></svg>
            </button>
          </div>
          {#if expandedAlbums.has(albumName)}
            <ul class="dl-track-list">
              {#each tracks as entry (entry.trackId)}
                <li class="dl-track-item">
                  <div class="dl-track-info">
                    <span class="dl-track-title">{entry.title}</span>
                    <span class="dl-track-meta">{entry.artistName || '—'} · {formatBytes(entry.sizeBytes)}</span>
                  </div>
                  <button class="dl-remove-btn" on:click={() => deleteDownload(entry.trackId)} title="Remove download">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/each}

      {#if filteredDone.length === 0 && dlSearch.trim()}
        <p class="msg" style="color:var(--text-2)">No downloads match "{dlSearch}"</p>
      {/if}
    {:else if activeEntries.length === 0 && errorEntries.length === 0 && abDoneCount === 0}
      <p class="msg" style="color:var(--text-2)">No tracks downloaded yet. Right-click any track and choose "Download offline".</p>
    {/if}

    <!-- ── Audiobooks ─────────────── -->
    {#if abDoneCount > 0}
      <div class="dl-section-head" style="margin-top: {doneCount > 0 ? '8px' : '0'}">
        <h3 class="dl-subhead">Audiobooks</h3>
        <span class="dl-section-stat">{audiobookGroups.length} book{audiobookGroups.length !== 1 ? 's' : ''} · {formatBytes(abTotalSizeBytes)}</span>
      </div>

      {#each audiobookGroups as [abKey, chapters] (abKey)}
        {@const bookTitle = chapters[0]?.albumName || 'Unknown Audiobook'}
        {@const bookAuthor = chapters[0]?.artistName || ''}
        <div class="dl-album-group">
          <div class="dl-album-header-row">
            <button class="dl-album-header" on:click={() => toggleABGroup(abKey)}>
              <svg class="dl-chevron" class:expanded={expandedABGroups.has(abKey)} width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>
              <span class="dl-album-name">{bookTitle}</span>
              <span class="dl-album-count">{bookAuthor ? bookAuthor + ' · ' : ''}{chapters.length} ch · {formatBytes(chapters.reduce((s, e) => s + e.sizeBytes, 0))}</span>
            </button>
            <button class="dl-remove-btn dl-remove-btn--album" on:click={() => deleteAlbumGroup(chapters)} title="Delete audiobook downloads">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14H6L5 6"/><path d="M10 11v6M14 11v6"/><path d="M9 6V4h6v2"/></svg>
            </button>
          </div>
          {#if expandedABGroups.has(abKey)}
            <ul class="dl-track-list">
              {#each chapters.sort((a, b) => a.title.localeCompare(b.title)) as entry (entry.trackId)}
                <li class="dl-track-item">
                  <div class="dl-track-info">
                    <span class="dl-track-title">{entry.title}</span>
                    <span class="dl-track-meta">{formatBytes(entry.sizeBytes)}</span>
                  </div>
                  <button class="dl-remove-btn" on:click={() => deleteDownload(entry.trackId)} title="Remove chapter">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/each}
    {/if}

    {#if doneCount + abDoneCount > 0}
      <div style="padding-top:8px;display:flex;align-items:center;gap:8px">
        {#if confirmDeleteAll}
          <span style="font-size:13px;color:var(--text-2)">Delete all downloads?</span>
          <button class="btn-danger" on:click={handleDeleteAll}>Confirm</button>
          <button class="btn-ghost" on:click={() => (confirmDeleteAll = false)}>Cancel</button>
        {:else}
          <button class="btn-danger" on:click={() => (confirmDeleteAll = true)}>Delete all downloads</button>
        {/if}
      </div>
    {/if}
  </section>

  <!-- ── About ─────────────────────────────────────────────── -->
  <section id="about" class="card">
    <h2 class="section-title">About</h2>
    {#if versionFetchError}
      <p class="msg" style="color:var(--text-2)">{versionFetchError}</p>
    {:else if serverVersion}
      <div class="about-item">
        <span class="about-label">Version</span>
        <span class="about-value">{serverVersion}</span>
      </div>
      {#if serverSha}
        <div class="about-item">
          <span class="about-label">Commit</span>
          <span class="about-value about-value--mono">{serverSha}</span>
        </div>
      {/if}
    {:else}
      <p class="msg" style="color:var(--text-2)">Loading version…</p>
    {/if}
  </section>
</div>
</div>

<svelte:head><title>Settings – Orb</title></svelte:head>

<style>
  /* ── Settings shell: desktop 2-col layout ── */
  .settings-shell {
    display: flex;
    align-items: flex-start;
    gap: 32px;
    max-width: 960px;
    margin: 0 auto;
    width: 100%;
  }

  .settings-nav {
    position: sticky;
    top: 24px;
    flex-shrink: 0;
    width: 148px;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding-top: 10px;
  }

  .settings-nav-title {
    font-size: 0.65rem;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--text-2);
    padding: 0 10px 8px;
  }

  .settings-nav-link {
    display: block;
    padding: 6px 10px;
    border-radius: 6px;
    font-size: 13px;
    color: var(--text-2);
    text-decoration: none;
    transition: background 0.15s, color 0.15s;
    white-space: nowrap;
  }
  .settings-nav-link:hover {
    background: var(--bg-2, rgba(255,255,255,0.06));
    color: var(--text);
  }

  .settings-nav-link.active {
    background: var(--accent, #6366f1);
    color: white;
    font-weight: 600;
  }

  /* On mobile: hide sidebar nav, full-width layout */
  @media (max-width: 640px) {
    .settings-shell {
      flex-direction: column;
      gap: 0;
    }
    .settings-nav {
      display: none;
    }
  }

  .page {
    flex: 1;
    min-width: 0;
    max-width: 720px;
    padding: 8px 0 32px;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .page-title {
    font-size: 1.5rem;
    font-weight: 700;
    color: var(--text);
    letter-spacing: -0.02em;
    margin-bottom: 4px;
  }

  /* ── Card ── */
  .card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 20px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .section-title {
    font-size: 0.8rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-2);
  }

  /* ── Toggle switch ── */
  .toggle-btn {
    position: relative;
    width: 40px;
    height: 22px;
    border-radius: 11px;
    border: 1px solid var(--border);
    background: var(--bg-2, var(--surface));
    cursor: pointer;
    padding: 0;
    flex-shrink: 0;
    transition: background 0.2s, border-color 0.2s;
  }
  .toggle-btn.on {
    background: var(--accent);
    border-color: var(--accent);
  }
  .toggle-knob {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: var(--text-muted);
    transition: transform 0.2s, background 0.2s;
  }
  .toggle-btn.on .toggle-knob {
    transform: translateX(18px);
    background: #fff;
  }

  /* ── Profile ── */
  .avatar-area {
    display: flex;
    align-items: center;
    gap: 16px;
  }

  .big-avatar {
    width: 72px;
    height: 72px;
    border-radius: 50%;
    background: linear-gradient(135deg, var(--accent), #818cf8);
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    overflow: hidden;
    box-shadow: 0 0 0 3px var(--accent-dim);
  }

  .avatar-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .avatar-initials {
    font-size: 22px;
    font-weight: 700;
    color: white;
    font-family: 'Syne', sans-serif;
  }

  .avatar-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .fields {
    display: flex;
    flex-direction: column;
    gap: 10px;
    border-top: 1px solid var(--border);
    padding-top: 14px;
  }

  .field {
    display: flex;
    align-items: baseline;
    gap: 12px;
  }

  .field-label {
    font-size: 12px;
    color: var(--text-2);
    width: 72px;
    flex-shrink: 0;
  }

  .field-value {
    font-size: 13px;
    color: var(--text);
    font-family: 'DM Mono', monospace;
  }

  .field-value--row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .badge {
    display: inline-flex;
    align-items: center;
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 11px;
    font-weight: 600;
    font-family: var(--font-sans, sans-serif);
    letter-spacing: 0.02em;
  }

  .badge--verified {
    background: rgba(34, 197, 94, 0.15);
    color: #22c55e;
  }

  .badge--unverified {
    background: rgba(234, 179, 8, 0.15);
    color: #eab308;
  }

  .verify-banner {
    margin-top: 12px;
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
    padding: 10px 14px;
    background: rgba(234, 179, 8, 0.08);
    border: 1px solid rgba(234, 179, 8, 0.2);
    border-radius: 8px;
  }

  .verify-banner__text {
    font-size: 13px;
    color: var(--text-2);
    flex: 1;
  }

  .btn--sm {
    padding: 5px 12px;
    font-size: 12px;
  }

  /* ── Form ── */
  .form-grid {
    display: grid;
    grid-template-columns: 140px 1fr;
    align-items: center;
    gap: 10px 12px;
  }

  .form-label {
    font-size: 12px;
    color: var(--text-2);
    text-align: right;
  }

  .form-input {
    width: 100%;
    height: 34px;
    background: var(--surface-2);
    border: 1px solid var(--border-2);
    border-radius: 7px;
    padding: 0 10px;
    color: var(--text);
    font-size: 13px;
    font-family: 'DM Mono', monospace;
    outline: none;
    transition: border-color 0.15s;
  }
  .form-input:focus { border-color: var(--accent); }
  .form-input:disabled { opacity: 0.5; }
  .form-input--error { border-color: #f87171 !important; }
  .form-input--textarea { height: auto; padding: 8px 10px; resize: vertical; font-family: inherit; line-height: 1.5; }
  .field-col { display: flex; flex-direction: column; gap: 3px; }
  .field-label-sm { font-size: 11px; font-weight: 600; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.04em; }
  .field-error { color: #f87171; font-size: 11px; }

  /* ── Messages ── */
  .msg {
    font-size: 12px;
    margin: 0;
  }
  .msg--error { color: #f87171; }
  .msg--ok    { color: #4ade80; }

  /* ── Buttons ── */
  .btn-primary {
    padding: 7px 16px;
    border-radius: 7px;
    border: none;
    background: var(--accent);
    color: #fff;
    font-size: 12px;
    font-weight: 600;
    font-family: 'Syne', sans-serif;
    cursor: pointer;
    transition: opacity 0.15s;
  }
  .btn-primary:hover:not(:disabled) { opacity: 0.85; }
  .btn-primary:disabled { opacity: 0.5; cursor: default; }

  .btn-ghost {
    padding: 7px 14px;
    border-radius: 7px;
    border: 1px solid var(--border-2);
    background: transparent;
    color: var(--text-2);
    font-size: 12px;
    font-family: 'Syne', sans-serif;
    cursor: pointer;
    transition: color 0.15s, border-color 0.15s;
  }
  .btn-ghost:hover { color: var(--text); border-color: var(--accent); }

  .btn-danger {
    padding: 7px 16px;
    border-radius: 7px;
    border: none;
    background: #ef4444;
    color: #fff;
    font-size: 12px;
    font-weight: 600;
    font-family: 'Syne', sans-serif;
    cursor: pointer;
    transition: opacity 0.15s;
  }
  .btn-danger:hover:not(:disabled) { opacity: 0.85; }
  .btn-danger:disabled { opacity: 0.5; cursor: default; }

  .btn-row {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  /* ── 2FA ── */
  .totp-hint {
    font-size: 12px;
    color: var(--text-2);
    line-height: 1.6;
    margin: 0;
  }

  .qr-wrap {
    display: flex;
    justify-content: center;
    padding: 12px 0;
  }

  .qr-img {
    border-radius: 8px;
    border: 1px solid var(--border);
    width: 180px;
    height: 180px;
  }

  .inline-secret {
    display: inline-block;
    background: var(--surface-2);
    border: 1px solid var(--border-2);
    border-radius: 4px;
    padding: 2px 6px;
    font-size: 11px;
    font-family: 'DM Mono', monospace;
    letter-spacing: 0.05em;
    word-break: break-all;
    color: var(--text);
    margin-top: 4px;
  }

  .backup-grid {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 8px;
  }

  .backup-code {
    background: var(--surface-2);
    border: 1px solid var(--border-2);
    border-radius: 6px;
    padding: 6px 0;
    text-align: center;
    font-family: 'DM Mono', monospace;
    font-size: 11px;
    letter-spacing: 0.06em;
    color: var(--text);
  }

  .totp-input {
    letter-spacing: 0.2em;
    text-align: center;
    font-size: 1.1rem;
    font-family: 'DM Mono', monospace;
  }

  /* ── Appearance ── */
  .setting-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 14px 0;
    border-top: 1px solid var(--border);
  }
  .setting-row:first-of-type { border-top: none; padding-top: 0; }
  .setting-row--col {
    flex-direction: column;
    align-items: flex-start;
  }

  .setting-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .setting-name {
    font-size: 13px;
    font-weight: 600;
    color: var(--text);
  }

  .setting-desc {
    font-size: 11px;
    color: var(--text-2);
  }

  .mode-toggle {
    display: flex;
    border: 1px solid var(--border-2);
    border-radius: 8px;
    overflow: hidden;
    flex-shrink: 0;
  }

  .mode-btn {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 14px;
    background: none;
    border: none;
    font-size: 12px;
    font-weight: 500;
    font-family: 'Syne', sans-serif;
    color: var(--text-2);
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }
  .mode-btn + .mode-btn { border-left: 1px solid var(--border-2); }
  .mode-btn.active { background: var(--accent-dim); color: var(--accent); }
  .mode-btn:hover:not(.active) { background: var(--surface-2); color: var(--text); }

  .palette {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    margin-top: 8px;
  }

  .swatch {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: var(--swatch);
    border: 2px solid transparent;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: transform 0.15s, box-shadow 0.15s, border-color 0.15s;
    flex-shrink: 0;
  }
  .swatch:hover { transform: scale(1.1); box-shadow: 0 0 0 4px var(--swatch-glow); }
  .swatch.selected { border-color: white; box-shadow: 0 0 0 3px var(--swatch-glow); transform: scale(1.05); }

  /* ── Streaming Quality ── */
  .sq-hint {
    font-size: 11px;
    color: var(--text-2);
    line-height: 1.6;
    margin: 0;
  }

  .sq-tab-note {
    font-size: 11px;
    color: var(--text-2);
    line-height: 1.5;
    margin: 0;
    padding: 8px 10px;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: 7px;
  }

  .sq-tabs {
    display: flex;
    border: 1px solid var(--border-2);
    border-radius: 8px;
    overflow: hidden;
  }

  .sq-tab {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 5px;
    padding: 7px 6px;
    background: none;
    border: none;
    font-size: 11.5px;
    font-weight: 500;
    font-family: 'Syne', sans-serif;
    color: var(--text-2);
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }
  .sq-tab + .sq-tab { border-left: 1px solid var(--border-2); }
  .sq-tab.active { background: var(--accent-dim); color: var(--accent); font-weight: 600; }
  .sq-tab:hover:not(.active) { background: var(--surface-2); color: var(--text); }

  .sq-presets {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px;
  }

  .sq-preset {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    padding: 12px 13px 10px;
    background: var(--surface-2);
    border: 1.5px solid var(--border-2);
    border-radius: 10px;
    cursor: pointer;
    text-align: left;
    transition: border-color 0.15s, background 0.15s, box-shadow 0.15s;
  }
  .sq-preset:hover:not(:disabled)  { border-color: var(--accent); }
  .sq-preset.active {
    border-color: var(--accent);
    background: var(--accent-dim);
    box-shadow: 0 0 0 1px var(--accent);
  }
  .sq-preset:disabled { opacity: 0.5; cursor: default; }

  .sq-preset-icon {
    color: var(--text-2);
    margin-bottom: 6px;
    line-height: 1;
  }
  .sq-preset.active .sq-preset-icon { color: var(--accent); }

  .sq-preset-name {
    font-size: 12.5px;
    font-weight: 700;
    color: var(--text);
    font-family: 'Syne', sans-serif;
    line-height: 1.2;
  }
  .sq-preset.active .sq-preset-name { color: var(--accent); }

  .sq-preset-desc {
    font-size: 11px;
    color: var(--text-2);
    line-height: 1.3;
  }

  .sq-preset-detail {
    font-size: 10px;
    color: var(--text-2);
    opacity: 0.65;
    font-family: 'DM Mono', monospace;
    line-height: 1.3;
    margin-top: 3px;
  }

  .sq-preset-badge {
    position: absolute;
    top: 8px;
    right: 8px;
    font-size: 9px;
    font-weight: 700;
    font-family: 'Syne', sans-serif;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    background: var(--accent);
    color: white;
    padding: 2px 6px;
    border-radius: 20px;
  }

  .sq-custom-btn {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 8px 12px;
    background: none;
    border: 1px dashed var(--border-2);
    border-radius: 8px;
    color: var(--text-2);
    font-size: 11.5px;
    font-family: 'Syne', sans-serif;
    cursor: pointer;
    transition: border-color 0.15s, color 0.15s, background 0.15s;
  }
  .sq-custom-btn:hover:not(:disabled) { border-color: var(--accent); color: var(--text); }
  .sq-custom-btn.open { border-color: var(--accent); color: var(--accent); background: var(--accent-dim); border-style: solid; }
  .sq-custom-btn:disabled { opacity: 0.5; cursor: default; }
  .sq-chevron { margin-left: auto; transition: transform 0.2s; }
  .sq-custom-btn.open .sq-chevron { transform: rotate(180deg); }

  .sq-custom-form {
    border: 1px solid var(--border-2);
    border-radius: 8px;
    padding: 4px 0;
    background: var(--surface-2);
    display: flex;
    flex-direction: column;
  }

  .sq-custom-row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 14px;
  }
  .sq-custom-row + .sq-custom-row { border-top: 1px solid var(--border); }

  .sq-custom-info {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .sq-custom-label {
    font-size: 12px;
    font-weight: 600;
    color: var(--text);
    font-family: 'Syne', sans-serif;
  }
  .sq-custom-label code {
    font-size: 10px;
    font-family: 'DM Mono', monospace;
    color: var(--text-2);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 3px;
    padding: 1px 4px;
    margin-left: 4px;
  }

  .sq-custom-sub {
    font-size: 10.5px;
    color: var(--text-2);
    line-height: 1.4;
  }

  .sq-custom-input {
    width: 110px;
    flex-shrink: 0;
    text-align: right;
    font-family: 'DM Mono', monospace;
  }

  .sq-transcode-row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 14px;
    border-top: 1px solid var(--border);
    margin-top: 8px;
  }
  .sq-transcode-select {
    width: 140px;
    flex-shrink: 0;
  }

  /* ── Mobile ─────────────────────────────────────────────── */
  @media (max-width: 640px) {
    .page { padding: 4px 0 24px; }

    /* Stack form labels above inputs */
    .form-grid {
      grid-template-columns: 1fr;
    }
    .form-label {
      text-align: left;
      padding-top: 4px;
    }

    /* Backup codes: 2 columns instead of 4 */
    .backup-grid {
      grid-template-columns: repeat(2, 1fr);
    }

    /* Settings rows: allow wrapping */
    .setting-row {
      flex-wrap: wrap;
    }
  }

  /* ── Device list ────────────────────────────────────────── */
  .device-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: 100%;
    margin-top: 10px;
  }

  .device-card {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 14px;
    border: 1.5px solid var(--border);
    border-radius: 10px;
    background: var(--surface);
    transition: border-color 0.15s, background 0.15s;
  }

  .device-card--active {
    border-color: var(--accent);
    background: var(--accent-dim);
  }

  .device-card-icon {
    color: var(--text-2);
    flex-shrink: 0;
    display: flex;
    align-items: center;
  }
  .device-card--active .device-card-icon { color: var(--accent); }

  .device-card-info {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
  }

  .device-card-name {
    font-size: 13px;
    font-weight: 600;
    color: var(--text);
    font-family: 'Syne', sans-serif;
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }

  .device-card-badge {
    font-size: 9.5px;
    font-weight: 700;
    font-family: 'Syne', sans-serif;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    background: var(--surface-2);
    color: var(--text-2);
    border: 1px solid var(--border);
    padding: 1px 6px;
    border-radius: 20px;
  }
  .device-card-badge--active {
    background: var(--accent);
    color: white;
    border-color: transparent;
  }

  .device-card-track {
    font-size: 11px;
    color: var(--text-2);
    display: flex;
    align-items: center;
    gap: 4px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* ── Downloads ── */
  .dl-summary {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
    margin-bottom: 8px;
  }
  .dl-stat {
    font-size: 13px;
    color: var(--text);
    font-weight: 500;
  }
  .dl-stat--muted {
    color: var(--text-2);
    font-weight: 400;
  }
  .dl-subhead {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-2);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    margin: 0 0 6px;
  }
  .dl-subhead--warn { color: var(--error, #ef4444); }

  .dl-active {
    margin-bottom: 12px;
  }
  .dl-active-item {
    padding: 6px 0;
  }
  .dl-active-info {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin-bottom: 4px;
  }
  .dl-active-title {
    font-size: 13px;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dl-active-meta {
    font-size: 11px;
    color: var(--text-2);
    flex-shrink: 0;
  }
  .dl-progress-bar {
    height: 3px;
    background: var(--border);
    border-radius: 2px;
    overflow: hidden;
  }
  .dl-progress-fill {
    height: 100%;
    background: var(--accent);
    border-radius: 2px;
    transition: width 0.2s ease;
  }

  .dl-errors {
    margin-bottom: 12px;
  }
  .dl-error-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 0;
    font-size: 12px;
  }
  .dl-error-title {
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dl-error-artist {
    color: var(--text-muted, #888);
    font-size: 11px;
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dl-error-msg {
    color: var(--error, #ef4444);
    font-size: 11px;
    flex-shrink: 0;
    max-width: 140px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .dl-search {
    width: 100%;
    padding: 7px 10px;
    font-size: 13px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-2, var(--bg));
    color: var(--text);
    outline: none;
    margin-bottom: 8px;
  }
  .dl-search:focus {
    border-color: var(--accent);
  }
  .dl-search::placeholder {
    color: var(--text-2);
  }

  .dl-album-group {
    border: 1px solid var(--border);
    border-radius: 6px;
    margin-bottom: 4px;
    overflow: hidden;
  }
  .dl-album-header {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 8px 10px;
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text);
    font-size: 13px;
    font-weight: 500;
    text-align: left;
  }
  .dl-album-header:hover {
    background: var(--bg-2, rgba(255,255,255,0.04));
  }
  .dl-chevron {
    flex-shrink: 0;
    color: var(--text-2);
    transition: transform 0.15s ease;
  }
  .dl-chevron.expanded {
    transform: rotate(90deg);
  }
  .dl-album-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dl-album-count {
    font-size: 11px;
    color: var(--text-2);
    font-weight: 400;
    flex-shrink: 0;
  }

  .dl-track-list {
    list-style: none;
    padding: 0;
    margin: 0;
  }
  .dl-track-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 5px 10px 5px 32px;
    border-top: 1px solid var(--border);
  }
  .dl-track-info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .dl-track-title {
    font-size: 12px;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dl-track-meta {
    font-size: 11px;
    color: var(--text-2);
  }
  .dl-remove-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    border: none;
    border-radius: 4px;
    background: none;
    color: var(--text-2);
    cursor: pointer;
  }
  .dl-remove-btn:hover {
    background: var(--bg-2, rgba(255,255,255,0.08));
    color: var(--error, #ef4444);
  }
  .dl-remove-btn--album {
    flex-shrink: 0;
    margin-right: 6px;
  }

  /* Album header row (chevron button + delete button side by side) */
  .dl-album-header-row {
    display: flex;
    align-items: center;
  }
  .dl-album-header-row .dl-album-header {
    flex: 1;
    min-width: 0;
  }

  /* Section sub-headings (Music / Audiobooks) */
  .dl-section-head {
    display: flex;
    align-items: baseline;
    gap: 10px;
    padding-bottom: 4px;
    border-bottom: 1px solid var(--border);
  }
  .dl-section-head .dl-subhead {
    margin: 0;
  }
  .dl-section-stat {
    font-size: 11px;
    color: var(--text-2);
    font-weight: 400;
  }

  /* ── About section ── */
  .about-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 0;
    border-bottom: 1px solid var(--border);
  }
  .about-item:last-child {
    border-bottom: none;
  }
  .about-label {
    font-size: 13px;
    color: var(--text-2);
    font-weight: 500;
  }
  .about-value {
    font-size: 13px;
    color: var(--text);
  }
  .about-value--mono {
    font-family: 'SF Mono', 'Monaco', 'Inconsolata', monospace;
    font-size: 12px;
    letter-spacing: 0.5px;
  }
</style>

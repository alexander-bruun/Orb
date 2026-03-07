import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
  appId: 'com.orb.app',
  appName: 'Orb',
  webDir: 'build',
  server: {
    androidScheme: 'http',
    allowNavigation: [],
  },
  ios: {
    contentInset: 'automatic',
  },
  android: {
    backgroundColor: '#080809',
  },
  plugins: {
    SplashScreen: {
      launchShowDuration: 800,
      backgroundColor: '#080809',
      androidSplashResourceName: 'splash',
      showSpinner: false,
    },
    StatusBar: {
      style: 'Dark',
      backgroundColor: '#080809',
    },
  },
};

export default config;

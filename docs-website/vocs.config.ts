import { defineConfig } from 'vocs/config'

export default defineConfig({
  title: 'TokenInsights',
  accentColor: 'light-dark(#285bd4, #82aaff)',
  description: 'Track token usage across OpenCode, Pi, Codex, and Claude Code.',
  baseUrl: 'https://tokeninsights.hoplabs.dev',
  logoUrl: { light: '/lockup-light.svg', dark: '/lockup-dark.svg' },
  iconUrl: '/favicon.svg',
  ogImageUrl: 'https://tokeninsights.hoplabs.dev/social-preview.png',
  renderStrategy: 'full-static',
  topNav: [
    { text: 'Docs', link: '/docs', match: '/docs' },
    { text: 'GitHub', link: 'https://github.com/flexdinesh/tokeninsights' },
  ],
  sidebar: [
    { text: 'Overview', link: '/docs' },
    { text: 'Install', link: '/docs/install' },
    { text: 'Getting started', link: '/docs/getting-started' },
    { text: 'Local usage', link: '/docs/local-usage' },
    { text: 'Commands', link: '/docs/commands' },
    { text: 'Remote connections', link: '/docs/remote' },
    { text: 'Maintenance', link: '/docs/maintenance' },
    { text: 'Privacy and data', link: '/docs/privacy' },
  ],
})

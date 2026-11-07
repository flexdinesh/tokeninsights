import { defineConfig } from 'vocs/config'

export default defineConfig({
  title: 'TokenInsights',
  description: 'Track token usage across OpenCode, Pi, Codex, and Claude Code.',
  baseUrl: 'https://tokeninsights.hoplabs.dev',
  logoUrl: { light: '/lockup-light.svg', dark: '/lockup-dark.svg' },
  iconUrl: '/favicon.svg',
  ogImageUrl: 'https://tokeninsights.hoplabs.dev/social-preview.png',
  renderStrategy: 'full-static',
  socials: [{ icon: 'github', link: 'https://github.com/flexdinesh/tokeninsights' }],
  sidebar: [
    { text: 'Overview', link: '/' },
    { text: 'Install', link: '/install' },
    { text: 'Getting started', link: '/getting-started' },
    { text: 'Local usage', link: '/local-usage' },
    { text: 'Commands', link: '/commands' },
    { text: 'Remote connections', link: '/remote' },
    { text: 'Maintenance', link: '/maintenance' },
    { text: 'Privacy and data', link: '/privacy' },
  ],
})

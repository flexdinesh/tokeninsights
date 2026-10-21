import { defineConfig } from 'vocs/config'

export default defineConfig({
  title: 'TokenInsights',
  description: 'Track token usage across OpenCode, Pi, Codex, and Claude Code.',
  renderStrategy: 'full-static',
  socials: [{ icon: 'github', link: 'https://github.com/flexdinesh/tokeninsights' }],
  sidebar: [
    { text: 'Overview', link: '/' },
    { text: 'Install', link: '/install' },
    { text: 'Getting started', link: '/getting-started' },
    { text: 'Local usage', link: '/local-usage' },
    { text: 'Commands', link: '/commands' },
    { text: 'Distributed sync', link: '/sync' },
    { text: 'Self-hosting', link: '/self-hosting' },
    { text: 'Maintenance', link: '/maintenance' },
    { text: 'Privacy and data', link: '/privacy' },
  ],
})

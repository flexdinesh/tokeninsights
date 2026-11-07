import { CopyInstall } from '../site/CopyInstall'
import { DashboardMock } from '../site/DashboardMock'
import { Logo } from '../site/Logo'

const GITHUB_URL = 'https://github.com/flexdinesh/tokeninsights'
const INSTALL_COMMAND =
  'curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install.sh | sh'
const DESCRIPTION =
  'TokenInsights tracks token usage across OpenCode, Pi, Codex and Claude Code. It runs on your machine and keeps only usage metadata.'
const FACTS = [
  {
    title: 'Stays on your machine',
    body: 'Usage history lives in SQLite where your agents run. No account or hosted service.',
  },
  {
    title: 'Metadata only',
    body: 'Token counts, models and sessions. Never prompts, responses or tool output.',
  },
  {
    title: 'Desktop, browser, terminal',
    body: 'The same numbers in each. Reach another machine over SSH when you ask to.',
  },
]

export default function Landing() {
  return (
    <div className="ti-body">
      <title>TokenInsights</title>
      <meta name="description" content={DESCRIPTION} />
      <div className="ti-page">
        <header className="ti-top">
          <a href="/" aria-label="TokenInsights home">
            <Logo />
          </a>
          <nav className="ti-links" aria-label="Main">
            <a href="/docs">Docs</a>
            <a href={GITHUB_URL}>GitHub</a>
          </nav>
        </header>
        <main>
          <section className="ti-hero">
            <h1>Token usage for your coding agents.</h1>
            <p className="ti-lead">{DESCRIPTION}</p>
            <div className="ti-install">
              <code>
                <span aria-hidden="true">$ </span>
                {INSTALL_COMMAND}
              </code>
              <CopyInstall command={INSTALL_COMMAND} />
            </div>
            <p className="ti-meta">
              macOS and Linux. MIT licensed. <a href="/docs">Read the docs</a>
            </p>
          </section>
          <figure className="ti-shot">
            <DashboardMock />
            <figcaption>The dashboard, with demo data.</figcaption>
          </figure>
          <section className="ti-facts" aria-label="What to expect">
            {FACTS.map((fact) => (
              <div key={fact.title}>
                <h2>{fact.title}</h2>
                <p>{fact.body}</p>
              </div>
            ))}
          </section>
        </main>
        <footer className="ti-foot">
          <span>TokenInsights</span>
          <nav className="ti-links" aria-label="Footer">
            <a href="/docs">Docs</a>
            <a href={GITHUB_URL}>GitHub</a>
          </nav>
        </footer>
      </div>
    </div>
  )
}

export const getConfig = async (): Promise<{ render: 'static' }> => ({ render: 'static' })

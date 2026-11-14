import { CopyInstall } from '../site/CopyInstall'
import { HeroDemo } from '../site/demos/HeroDemo'
import { RemoteDemo } from '../site/demos/RemoteDemo'
import { Logo } from '../site/Logo'

const GITHUB_URL = 'https://github.com/flexdinesh/tokeninsights'
const INSTALL_COMMAND =
  'curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install.sh | sh'
const DESCRIPTION =
  'TokenInsights tracks token usage across OpenCode, Pi, Codex and Claude Code. It runs on your machine and keeps only usage metadata.'
const REMOTE_STEPS = [
  {
    title: 'Over SSH',
    commands: ['tokeninsights connect ssh://you@devbox'],
    body: 'Your SSH keys and host verification. It starts the backend on devbox when it is not running.',
  },
  {
    title: 'Without SSH',
    commands: ['tokeninsights remote', "tokeninsights connect 'tokeninsights://connect/v1?…'"],
    body: 'Run remote on devbox. It prints a connect command with a pinned key and a temporary token; run that on your laptop. Stopping remote revokes it.',
  },
]
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
            <div
              role="img"
              aria-label="The TokenInsights desktop app with demo data. Refresh is pressed and today's new usage rolls into the totals, the chart and the table."
            >
              <HeroDemo />
            </div>
            <figcaption>The desktop app, with demo data.</figcaption>
          </figure>
          <section className="ti-remote" aria-labelledby="ti-remote-title">
            <div className="ti-remote-head">
              <h2 id="ti-remote-title">Agents on another machine?</h2>
              <p className="ti-remote-lead">
                Usage stays in that machine's SQLite. <code>connect</code> opens it in the desktop
                app on your laptop. Nothing is copied or merged, and the window says which machine
                you are looking at.
              </p>
            </div>
            <div className="ti-remote-copy">
              <ol className="ti-remote-steps">
                {REMOTE_STEPS.map((step) => (
                  <li key={step.title}>
                    <h3>{step.title}</h3>
                    <pre>
                      {step.commands.map((command) => (
                        <code key={command}>
                          <span aria-hidden="true">$ </span>
                          {command}
                        </code>
                      ))}
                    </pre>
                    <p>{step.body}</p>
                  </li>
                ))}
              </ol>
              <a className="ti-remote-more" href="/docs/remote">
                Remote connections
              </a>
            </div>
            <div
              className="ti-remote-shot"
              role="img"
              aria-label="On a laptop, a terminal runs tokeninsights connect ssh://you@devbox, then pastes the connect command printed by tokeninsights remote on devbox. Each opens a desktop window over the laptop's own, connected to devbox over SSH and then TCP, and devbox's usage loads into it. Demo data."
            >
              <RemoteDemo />
            </div>
          </section>
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

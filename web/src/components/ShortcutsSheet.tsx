import { Modal } from './ui'

const GROUPS: { title: string; keys: [string[], string][] }[] = [
  { title: 'Go to', keys: [[['h'], 'Today'], [['r'], 'Routes'], [['o'], 'Outings'], [['m'], 'Map'], [['g'], 'Gear'], [['y'], 'Your year'], [[','], 'Settings']] },
  { title: 'Make', keys: [[['n'], 'Plan an outing'], [['i'], 'Bring in a route (GPX, KML…)'], [['d'], 'Draw a route'], [['a'], 'Add gear']] },
  { title: 'Find', keys: [[['/'], 'Search & commands'], [['⌘', 'K'], 'Search & commands']] },
  { title: 'Window', keys: [[['['], 'Show or hide the sidebar'], [['esc'], 'Close a dialog or menu']] },
  { title: 'Help', keys: [[['?'], 'This sheet']] },
]

export function ShortcutsSheet({ onClose }: { onClose: () => void }) {
  return (
    <Modal title="Keyboard shortcuts" onClose={onClose} size="lg">
      <div className="shortcuts">
        {GROUPS.map(g => (
          <section key={g.title}>
            <h4 className="sc-h">{g.title}</h4>
            {g.keys.map(([keys, label]) => (
              <div key={label + keys.join('')} className="sc-row"><span>{label}</span><span className="sc-keys">{keys.map(k => <kbd key={k}>{k}</kbd>)}</span></div>
            ))}
          </section>
        ))}
      </div>
    </Modal>
  )
}

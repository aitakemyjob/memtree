import { Plug } from 'lucide-react';
import { useStore } from '../lib/store';
import { ConnectPanel } from './ConnectPanel';
import { Modal } from './ui';

/** Dialog with MCP connection snippets and a shortcut to the hooks installer. */
export function ConnectAgent() {
  const s = useStore();
  const close = () => s.setConnectOpen(false);
  return (
    <Modal
      title="Connect an agent"
      icon={<Plug size={16} style={{ color: 'var(--accent-text)' }} />}
      onClose={close}
      wide
      footer={
        <>
          <button className="btn" onClick={() => { close(); s.openView('settings', { section: 'hooks' }); }}>Install hooks…</button>
          <button className="btn primary" onClick={close}>Done</button>
        </>
      }
    >
      <ConnectPanel />
    </Modal>
  );
}

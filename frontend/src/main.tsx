import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './styles/global.css';
import { applyTheme, cachedThemeId, themeById } from './lib/themes';
import { App } from './App';

// Paint the remembered theme before the backend answers so a light-theme
// user never sees a dark flash (the shell window itself is SFC dark).
applyTheme(themeById(cachedThemeId()));

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

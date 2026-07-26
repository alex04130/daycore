// Entry: boot the design system (globals first), init the session, render.
import './boot/ds.js';
import './app.css';
import React from 'react';
import { createRoot } from 'react-dom/client';
import S from './store.js';
import { ToastProvider, applyCurrentTheme } from './ui.jsx';
import App from './App.jsx';

const root = createRoot(document.getElementById('root'));

S.bootstrap()
  .catch((err) => { console.error('bootstrap failed', err); })
  .finally(() => {
    applyCurrentTheme(S.state);
    root.render(
      <React.StrictMode>
        <ToastProvider><App /></ToastProvider>
      </React.StrictMode>
    );
  });

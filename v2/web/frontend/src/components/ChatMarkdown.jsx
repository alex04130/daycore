// Markdown/LaTeX renderer for assistant chat bubbles. The heavy pipeline
// (react-markdown + katex) is code-split via React.lazy so it only loads once
// a chat message renders; until then the raw text shows as fallback.
import React, { Suspense } from 'react';

const Impl = React.lazy(() => import('./ChatMarkdownImpl.jsx'));

export default function ChatMarkdown({ children }) {
  return (
    <Suspense fallback={children}>
      <Impl>{children}</Impl>
    </Suspense>
  );
}

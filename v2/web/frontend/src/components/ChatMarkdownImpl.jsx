import React from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import rehypeKatex from 'rehype-katex';
import 'katex/dist/katex.min.css';

const remarkPlugins = [remarkGfm, remarkMath];
const rehypePlugins = [rehypeKatex];
const components = {
  a: ({ node, ...props }) => <a target="_blank" rel="noreferrer" {...props} />,
  // wide tables scroll inside the bubble instead of blowing out its width
  table: ({ node, ...props }) => <div className="dc-md-scroll"><table {...props} /></div>,
};

export default function ChatMarkdownImpl({ children }) {
  return (
    <div className="dc-md">
      <ReactMarkdown remarkPlugins={remarkPlugins} rehypePlugins={rehypePlugins} components={components}>
        {children || ''}
      </ReactMarkdown>
    </div>
  );
}

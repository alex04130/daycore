import React from 'react';

// Inline SVG icons (lucide-style strokes). Paths are copied verbatim from the
// design prototype: design-ui/liuli/admin/admin-icons.js and
// design-ui/liuli/app/icons.js + icons-ext.js.
const C = (cx, cy, r) => ({ tag: 'circle', attrs: { cx, cy, r } });
const R = (x, y, w, h, rx) => ({ tag: 'rect', attrs: { x, y, width: w, height: h, rx } });
const E = (cx, cy, rx, ry) => ({ tag: 'ellipse', attrs: { cx, cy, rx, ry } });

function make(paths, extra) {
  return function Icon({ size = 20, className, style, strokeWidth = 2 }) {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth={strokeWidth}
        strokeLinecap="round"
        strokeLinejoin="round"
        className={className}
        style={style}
        aria-hidden="true"
      >
        {paths.split('|').filter(Boolean).map((d, i) => (
          <path key={'p' + i} d={d} />
        ))}
        {(extra || []).map((n, i) =>
          React.createElement(n.tag, { key: 'x' + i, ...n.attrs }),
        )}
      </svg>
    );
  };
}

export const Gauge = make('m12 14 4-4|M3.34 19a10 10 0 1 1 17.32 0');
export const FileText = make('M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7z|M14 2v4a2 2 0 0 0 2 2h4|M16 13H8|M16 17H8|M10 9H8');
export const ScrollText = make('M15 12h-5|M15 8h-5|M19 17V5a2 2 0 0 0-2-2H4|M8 21h12a2 2 0 0 0 2-2v-1a1 1 0 0 0-1-1H11a1 1 0 0 0-1 1v1a2 2 0 1 1-4 0V5a2 2 0 1 0-4 0v2a1 1 0 0 0 1 1h3');
export const Users = make('M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2|M22 21v-2a4 4 0 0 0-3-3.87|M16 3.13a4 4 0 0 1 0 7.75', [C(9, 7, 4)]);
export const User = make('M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2', [C(12, 7, 4)]);
export const Sparkles = make('M9.9 2.9 11.5 7l4.1 1.6-4.1 1.6-1.6 4.1-1.6-4.1L4.2 8.6l4.1-1.6 1.6-4.1z|M19 12l.9 2.3 2.3.9-2.3.9-.9 2.3-.9-2.3-2.3-.9 2.3-.9.9-2.3z|M6.5 16.5l.7 1.8 1.8.7-1.8.7-.7 1.8-.7-1.8-1.8-.7 1.8-.7.7-1.8z');
export const Zap = make('M13 2 3 14h9l-1 8 10-12h-9l1-8z');
export const Heart = make('M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7z');
export const Database = make('M3 5v14a9 3 0 0 0 18 0V5|M3 12a9 3 0 0 0 18 0', [E(12, 5, 9, 3)]);
export const Download = make('M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4|m7 10 5 5 5-5|M12 15V3');
export const AlertTriangle = make('m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3|M12 9v4|M12 17h.01');
export const ShieldCheck = make('M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z|m9 12 2 2 4-4');
export const Server = make('M6 6h.01|M6 18h.01', [R(2, 2, 20, 8, 2), R(2, 14, 20, 8, 2)]);
export const Sliders = make('M21 4h-7|M10 4H3|M21 12h-9|M8 12H3|M21 20h-5|M12 20H3|M14 2v4|M8 10v4|M16 18v4');
export const Cpu = make('M15 2v2|M15 20v2|M2 15h2|M2 9h2|M20 15h2|M20 9h2|M9 2v2|M9 20v2', [R(4, 4, 16, 16, 2), R(9, 9, 6, 6, 1)]);
export const KeyRound = make('M2.586 17.414A2 2 0 0 0 2 18.828V21a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h1a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h.172a2 2 0 0 0 1.414-.586l.814-.814a6.5 6.5 0 1 0-4-4z', [C(16.5, 7.5, 0.7)]);
export const Plus = make('M5 12h14|M12 5v14');
export const Trash = make('M3 6h18|M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6|M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2|M10 11v6|M14 11v6');
export const X = make('M18 6 6 18|m6 6 12 12');
export const ChevronLeft = make('m15 18-6-6 6-6');
export const ChevronRight = make('m9 18 6-6-6-6');
export const Upload = make('M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4|m17 8-5-5-5 5|M12 3v12');
export const FileJson = make('M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z|M14 2v6h6|M10 12a1 1 0 0 0-1 1v1a1 1 0 0 1-1 1 1 1 0 0 1 1 1v1a1 1 0 0 0 1 1|M14 18a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1 1 1 0 0 1-1-1v-1a1 1 0 0 0-1-1');
export const ExternalLink = make('M15 3h6v6|M10 14 21 3|M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6');
export const LogOut = make('M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4|m16 17 5-5-5-5|M21 12H9');

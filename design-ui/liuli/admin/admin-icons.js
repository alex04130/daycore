// Daycore Admin — extra console icons (lucide-style)
(function () {
  'use strict';
  const e = React.createElement;
  function I(d, extra) {
    return function Icon(props) {
      const { size = 20, className, style, strokeWidth = 2 } = props || {};
      const kids = [];
      d.split('|').forEach((seg, i) => { if (seg) kids.push(e('path', { key: 'p' + i, d: seg })); });
      (extra || []).forEach((n, i) => kids.push(e(n.tag, Object.assign({ key: 'x' + i }, n.attrs))));
      return e('svg', { width: size, height: size, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth, strokeLinecap: 'round', strokeLinejoin: 'round', className, style }, kids);
    };
  }
  const C = (cx, cy, r) => ({ tag: 'circle', attrs: { cx, cy, r } });
  const R = (x, y, w, h, rx) => ({ tag: 'rect', attrs: { x, y, width: w, height: h, rx } });
  const E = (cx, cy, rx, ry) => ({ tag: 'ellipse', attrs: { cx, cy, rx, ry } });
  Object.assign(window.DcIcons, {
    Gauge: I('m12 14 4-4|M3.34 19a10 10 0 1 1 17.32 0'),
    FileText: I('M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7z|M14 2v4a2 2 0 0 0 2 2h4|M16 13H8|M16 17H8|M10 9H8'),
    ScrollText: I('M15 12h-5|M15 8h-5|M19 17V5a2 2 0 0 0-2-2H4|M8 21h12a2 2 0 0 0 2-2v-1a1 1 0 0 0-1-1H11a1 1 0 0 0-1 1v1a2 2 0 1 1-4 0V5a2 2 0 1 0-4 0v2a1 1 0 0 0 1 1h3'),
    Users: I('M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2|M22 21v-2a4 4 0 0 0-3-3.87|M16 3.13a4 4 0 0 1 0 7.75', [C(9, 7, 4)]),
    Database: I('M3 5v14a9 3 0 0 0 18 0V5|M3 12a9 3 0 0 0 18 0', [E(12, 5, 9, 3)]),
    Download: I('M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4|m7 10 5 5 5-5|M12 15V3'),
    AlertTriangle: I('m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3|M12 9v4|M12 17h.01'),
    ShieldCheck: I('M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z|m9 12 2 2 4-4'),
    Server: I('M6 6h.01|M6 18h.01', [R(2, 2, 20, 8, 2), R(2, 14, 20, 8, 2)]),
    Sliders: I('M21 4h-7|M10 4H3|M21 12h-9|M8 12H3|M21 20h-5|M12 20H3|M14 2v4|M8 10v4|M16 18v4'),
    Cpu: I('M15 2v2|M15 20v2|M2 15h2|M2 9h2|M20 15h2|M20 9h2|M9 2v2|M9 20v2', [R(4, 4, 16, 16, 2), R(9, 9, 6, 6, 1)]),
    KeyRound: I('M2.586 17.414A2 2 0 0 0 2 18.828V21a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h1a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h.172a2 2 0 0 0 1.414-.586l.814-.814a6.5 6.5 0 1 0-4-4z', [C(16.5, 7.5, 0.7)]),
    Plus: I('M5 12h14|M12 5v14'),
  });
})();

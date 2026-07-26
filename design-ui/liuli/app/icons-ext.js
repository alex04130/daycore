// Daycore v2.2 — additional icons (same lucide-style factory as icons.js)
(function () {
  'use strict';
  const e = React.createElement;
  function I(d, extra) {
    return function Icon(props) {
      const { size = 20, className, style, strokeWidth = 2 } = props || {};
      const kids = [];
      d.split('|').forEach((seg, i) => { if (seg) kids.push(e('path', { key: 'p' + i, d: seg })); });
      (extra || []).forEach((n, i) => kids.push(e(n.tag, Object.assign({ key: 'x' + i }, n.attrs))));
      return e('svg', {
        width: size, height: size, viewBox: '0 0 24 24', fill: 'none',
        stroke: 'currentColor', strokeWidth, strokeLinecap: 'round', strokeLinejoin: 'round',
        className, style, 'aria-hidden': true,
      }, kids);
    };
  }
  const C = (cx, cy, r) => ({ tag: 'circle', attrs: { cx, cy, r } });
  const R = (x, y, w, h, rx) => ({ tag: 'rect', attrs: { x, y, width: w, height: h, rx } });

  Object.assign(window.DcIcons, {
    Camera: I('M14.5 4h-5L7.5 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3.5L14.5 4z', [C(12, 13, 3)]),
    Search: I('m21 21-4.3-4.3', [C(11, 11, 7)]),
    Salad: I('M7 21h10|M12 21a9 9 0 0 0 9-9H3a9 9 0 0 0 9 9z|M11.38 12a2.4 2.4 0 0 1-.4-4.77 2.4 2.4 0 0 1 3.2-2.77 2.4 2.4 0 0 1 3.47-.63 2.4 2.4 0 0 1 3.37 3.37 2.4 2.4 0 0 1-1.1 3.7|m13 12 4-4|M10.9 7.25A3.99 3.99 0 0 0 4 10c0 .73.2 1.41.54 2'),
    HeartPulse: I('M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7z|M3.22 12H9.5l.5-1 2 4.5 2-7 1.5 3.5h5.27'),
    Plane: I('M17.8 19.2 16 11l3.5-3.5C21 6 21.5 4 21 3c-1-.5-3 0-4.5 1.5L13 8 4.8 6.2c-.5-.1-.9.1-1.1.5l-.3.5c-.2.5-.1 1 .3 1.3L9 12l-2 3H4l-1 1 3 2 2 3 1-1v-3l3-2 3.5 5.3c.3.4.8.5 1.3.3l.5-.2c.4-.3.6-.7.5-1.2z'),
    Wallet: I('M19 7V4a1 1 0 0 0-1-1H5a2 2 0 0 0 0 4h15a1 1 0 0 1 1 1v4h-3a2 2 0 0 0 0 4h3a1 1 0 0 0 1-1v-2|M3 5v14a2 2 0 0 0 2 2h15a1 1 0 0 0 1-1v-4'),
    Lightbulb: I('M15 14c.2-1 .7-1.7 1.5-2.5 1-.9 1.5-2.2 1.5-3.5A6 6 0 0 0 6 8c0 1.3.5 2.6 1.5 3.5.8.8 1.3 1.5 1.5 2.5|M9 18h6|M10 22h4'),
    ShoppingBag: I('M6 2 3 6v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4z|M3 6h18|M16 10a4 4 0 0 1-8 0'),
    Film: I('M7 3v18|M17 3v18|M3 7.5h4|M3 16.5h4|M17 7.5h4|M17 16.5h4|M3 12h18', [R(3, 3, 18, 18, 2)]),
    NotebookPen: I('M13.4 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-7.6|M2 6h4|M2 10h4|M2 14h4|M2 18h4|M18.4 2.6a2.17 2.17 0 0 1 3 3L16 11l-4 1 1-4z'),
    History: I('M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8|M3 3v5h5|M12 7v5l4 2'),
    Undo2: I('M9 14 4 9l5-5|M4 9h10.5a5.5 5.5 0 0 1 5.5 5.5 5.5 5.5 0 0 1-5.5 5.5H11'),
    MoreHorizontal: I('', [C(5, 12, 1), C(12, 12, 1), C(19, 12, 1)]),
    Brain: I('M12 5a3 3 0 1 0-5.997.125 4 4 0 0 0-2.526 5.77 4 4 0 0 0 .556 6.588A4 4 0 1 0 12 18z|M12 5a3 3 0 1 1 5.997.125 4 4 0 0 1 2.526 5.77 4 4 0 0 1-.556 6.588A4 4 0 1 1 12 18z|M12 5v13'),
    Info: I('M12 16v-4|M12 8h.01', [C(12, 12, 10)]),
    MessageCircle: I('M7.9 20A9 9 0 1 0 4 16.1L2 22z'),
    MessagesSquare: I('M14 9a2 2 0 0 1-2 2H6l-4 4V4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2z|M18 9h2a2 2 0 0 1 2 2v11l-4-4h-6a2 2 0 0 1-2-2v-1'),
    Route: I('M9 19h8.5a3.5 3.5 0 0 0 0-7h-11a3.5 3.5 0 0 1 0-7H15', [C(6, 19, 3), C(18, 5, 3)]),
  });
})();

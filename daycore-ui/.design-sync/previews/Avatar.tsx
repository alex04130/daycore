import { Avatar } from "@daycore/ui";

// A self-contained data-URI image so the image path renders offline.
const IMG =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    `<svg xmlns='http://www.w3.org/2000/svg' width='80' height='80'><defs><linearGradient id='g' x1='0' y1='0' x2='1' y2='1'><stop offset='0' stop-color='%2360a5fa'/><stop offset='1' stop-color='%233b82f6'/></linearGradient></defs><rect width='80' height='80' fill='url(%23g)'/><circle cx='40' cy='30' r='14' fill='white' opacity='0.9'/><rect x='16' y='50' width='48' height='26' rx='13' fill='white' opacity='0.9'/></svg>`,
  );

export function Initials() {
  return (
    <div className="flex items-center gap-3">
      <Avatar name="Alex" />
      <Avatar name="林晓" />
      <Avatar fallback="leo@daycore.app" />
      <Avatar name="?" />
    </div>
  );
}

export function Sizes() {
  return (
    <div className="flex items-center gap-3">
      <Avatar name="S" size={28} />
      <Avatar name="M" size={40} />
      <Avatar name="L" size={56} />
    </div>
  );
}

export function WithImage() {
  return (
    <div className="flex items-center gap-3">
      <Avatar src={IMG} name="Daycore" size={40} />
      <Avatar src={IMG} name="Daycore" size={56} />
    </div>
  );
}

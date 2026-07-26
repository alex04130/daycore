import { ThemeSwitcher } from "@daycore/ui";

export function Pill() {
  return (
    <div className="w-72">
      <ThemeSwitcher value="sky" />
    </div>
  );
}

export function PillNight() {
  return (
    <div className="w-72" data-theme="night">
      <ThemeSwitcher value="night" />
    </div>
  );
}

export function Icon() {
  return <ThemeSwitcher value="sunset" variant="icon" />;
}

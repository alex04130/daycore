import { ThemeProvider, GlassCard, Button, Chip, type ThemeId } from "@daycore/ui";

function Scene({ name }: { name: string }) {
  return (
    <GlassCard className="dc-app-bg w-44" padding="md">
      <p className="text-sm font-semibold" style={{ color: "var(--color-text-primary)" }}>
        {name}
      </p>
      <p className="mb-3 mt-0.5 text-xs" style={{ color: "var(--color-text-muted)" }}>
        同一套组件，换肤即变
      </p>
      <div className="flex items-center gap-2">
        <Button size="sm">主要</Button>
        <Chip variant="selected">标签</Chip>
      </div>
    </GlassCard>
  );
}

function Demo({ theme, label }: { theme: ThemeId; label: string }) {
  return (
    <ThemeProvider theme={theme} target="self">
      <Scene name={label} />
    </ThemeProvider>
  );
}

export function AllThemes() {
  return (
    <div className="flex flex-wrap gap-4">
      <Demo theme="sky" label="天空蓝" />
      <Demo theme="sunset" label="暖橙日落" />
      <Demo theme="night" label="深夜紫" />
      <Demo theme="nature" label="自然绿" />
    </div>
  );
}

// Push — single source of truth for brand assets.
// Swap APP_ICON.src here (or via the Tweaks panel at runtime) to repaint
// the brand mark across the rail, sidebar header, and page favicon.

const APP_ICON = {
  src: 'icon.svg',          // path or data: URL
  alt: 'Push',
  // The icon's own background — set to 'transparent' if your icon is a
  // standalone glyph that should sit on the surface tile.
  bg: '#000',
  // How the icon image is laid into the tile (ratio of inner image size to tile size).
  innerScale: 0.88,
  // Corner radius applied to the tile container.
  radius: 9,
};

// Update favicon link tag whenever icon src changes.
function applyFavicon(src) {
  let link = document.querySelector('link[rel="icon"]');
  if (!link) {
    link = document.createElement('link');
    link.rel = 'icon';
    document.head.appendChild(link);
  }
  link.href = src;
}

// Reusable brand-tile component. Renders APP_ICON inside a colored tile.
// Supports overrides for size, radius, showing a status pulse dot.
function BrandMark({ icon = APP_ICON, size = 32, theme, pulse = false, style = {} }) {
  const inner = Math.round(size * (icon.innerScale ?? 0.88));
  return (
    <div style={{
      width: size, height: size, borderRadius: icon.radius ?? 9,
      background: icon.bg ?? '#000',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      boxShadow: `0 ${size >= 32 ? 6 : 4}px ${size >= 32 ? 18 : 12}px rgba(0,0,0,0.45), 0 0 0 1px ${theme?.borderStrong ?? 'rgba(255,255,255,0.14)'}`,
      position: 'relative', overflow: 'visible', flexShrink: 0,
      ...style,
    }}>
      <img
        src={icon.src} alt={icon.alt}
        width={inner} height={inner}
        style={{ display: 'block', borderRadius: Math.max(0, (icon.radius ?? 9) - 3), objectFit: 'contain' }}
      />
      {pulse && theme && (
        <span style={{
          position: 'absolute', top: -3, right: -3,
          width: 8, height: 8, borderRadius: 999,
          background: theme.ok,
          boxShadow: `0 0 0 2px ${theme.bg}, 0 0 8px ${theme.ok}`,
          animation: 'pushPulse 2s infinite',
        }} />
      )}
    </div>
  );
}

Object.assign(window, { APP_ICON, BrandMark, applyFavicon });

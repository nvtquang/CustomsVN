import { useState } from "react";

/**
 * One measure per day as thin bars on a zero baseline: one series, one hue
 * (the primary token), so the title names it and no legend is needed. Hover
 * or focus a day for its value; the table view on the page holds every
 * number for those who cannot see the chart.
 */
export function BarChart({
  title,
  days,
  values,
  format,
}: {
  title: string;
  days: string[];
  values: number[];
  format: (v: number) => string;
}) {
  const [hover, setHover] = useState<number | null>(null);
  const w = 600;
  const h = 140;
  const pad = { top: 8, bottom: 18, left: 4, right: 4 };
  const max = Math.max(...values, 0);
  const plotH = h - pad.top - pad.bottom;
  const step = (w - pad.left - pad.right) / Math.max(values.length, 1);
  const bar = Math.max(2, Math.min(step - 2, 18)); // 2px surface gap between bars
  const y = (v: number) => pad.top + plotH - (max > 0 ? (v / max) * plotH : 0);
  const i = hover;
  return (
    <figure className="space-y-1">
      <figcaption className="flex items-baseline justify-between text-xs">
        <span className="font-medium">{title}</span>
        <span className="text-muted-foreground tabular-nums">
          {i != null ? `${days[i]}: ${format(values[i])}` : max > 0 ? `max ${format(max)}` : ""}
        </span>
      </figcaption>
      <svg
        viewBox={`0 0 ${w} ${h}`}
        className="h-36 w-full"
        role="img"
        aria-label={title}
        onMouseLeave={() => setHover(null)}
      >
        <line
          x1={pad.left}
          x2={w - pad.right}
          y1={pad.top + plotH}
          y2={pad.top + plotH}
          className="stroke-border"
          strokeWidth={1}
        />
        {values.map((v, k) => {
          const x = pad.left + k * step + (step - bar) / 2;
          const top = y(v);
          const hgt = pad.top + plotH - top;
          return (
            <g key={days[k]} onMouseEnter={() => setHover(k)}>
              {/* Hit target: the whole column, wider than the bar. */}
              <rect x={pad.left + k * step} y={pad.top} width={step} height={plotH} fill="transparent" />
              {hgt > 0 && (
                <path
                  d={roundedTop(x, top, bar, hgt, Math.min(4, bar / 2, hgt))}
                  className={k === hover ? "fill-primary" : "fill-primary/70"}
                />
              )}
            </g>
          );
        })}
        {[0, values.length - 1].map((k) =>
          values.length > 0 ? (
            <text
              key={k}
              x={k === 0 ? pad.left : w - pad.right}
              y={h - 4}
              textAnchor={k === 0 ? "start" : "end"}
              className="fill-muted-foreground text-2xs"
            >
              {days[k]?.slice(5)}
            </text>
          ) : null,
        )}
      </svg>
    </figure>
  );
}

/** A bar with its data end rounded and its base square on the axis. */
function roundedTop(x: number, y: number, w: number, h: number, r: number) {
  return `M${x},${y + h} V${y + r} Q${x},${y} ${x + r},${y} H${x + w - r} Q${x + w},${y} ${x + w},${y + r} V${y + h} Z`;
}

interface Props {
  grid: string[];
  palette: Record<string, string>;
  x?: number;
  y?: number;
}

export default function PixelSprite({ grid, palette, x = 0, y = 0 }: Props) {
  const rects = [];
  for (let row = 0; row < grid.length; row++) {
    const line = grid[row];
    let col = 0;
    while (col < line.length) {
      const ch = line[col];
      let end = col + 1;
      while (end < line.length && line[end] === ch) end++;
      if (ch !== "." && palette[ch]) {
        rects.push(
          <rect key={`${row}-${col}`} x={x + col} y={y + row} width={end - col} height={1} fill={palette[ch]} />,
        );
      }
      col = end;
    }
  }
  return <g shapeRendering="crispEdges">{rects}</g>;
}

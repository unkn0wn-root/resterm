// The empty response pane logo from internal/ui/model_core.go. Each character
// cell holds two square pixels: the top and bottom half of a block.
const art = ['░█▀▄░█▀▀░█▀▀░▀█▀░█▀▀░█▀▄░█▄█', '░█▀▄░█▀▀░▀▀█░░█░░█▀▀░█▀▄░█░█', '░▀░▀░▀▀▀░▀▀▀░░▀░░▀▀▀░▀░▀░▀░▀'];

const halves: Record<string, [top: boolean, bottom: boolean]> = {
  '█': [true, true],
  '▀': [true, false],
  '▄': [false, true],
};

export interface Pixel {
  x: number;
  y: number;
}

export interface Wordmark {
  width: number;
  height: number;
  pixels: Pixel[];
  shade: Pixel[];
}

export function wordmark(): Wordmark {
  const pixels: Pixel[] = [];
  const shade: Pixel[] = [];
  art.forEach((line, row) => {
    [...line].forEach((ch, x) => {
      if (ch === '░') {
        shade.push({ x, y: row * 2 });
        return;
      }
      const [top, bottom] = halves[ch] ?? [false, false];
      if (top) pixels.push({ x, y: row * 2 });
      if (bottom) pixels.push({ x, y: row * 2 + 1 });
    });
  });
  return { width: [...art[0]].length, height: art.length * 2, pixels, shade };
}

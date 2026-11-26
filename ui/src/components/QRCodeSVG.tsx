import React, { useMemo } from 'react';
import { encodeQR } from '../lib/qrcode';

interface QRCodeSVGProps {
  value: string;
  size?: number;
  label?: string;
}

/**
 * Renders a crisp monochrome SVG QR code with a quiet zone for authenticator apps.
 */
export const QRCodeSVG: React.FC<QRCodeSVGProps> = ({ value, size = 160, label }) => {
  const { modules, pathData } = useMemo(() => {
    const matrix = encodeQR(value);
    const n = matrix.length;
    const parts: string[] = [];
    for (let r = 0; r < n; r++) {
      for (let c = 0; c < n; c++) {
        if (matrix[r][c]) {
          parts.push(`M${c},${r}h1v1h-1z`);
        }
      }
    }
    return { modules: n, pathData: parts.join('') };
  }, [value]);

  const quiet = 2;
  const viewBoxSize = modules + quiet * 2;

  return (
    <svg
      width={size}
      height={size}
      viewBox={`${-quiet} ${-quiet} ${viewBoxSize} ${viewBoxSize}`}
      shapeRendering="crispEdges"
      role="img"
      aria-label={label}
      className="block"
    >
      <rect x={-quiet} y={-quiet} width={viewBoxSize} height={viewBoxSize} fill="#ffffff" />
      <path d={pathData} fill="#0f172a" />
    </svg>
  );
};

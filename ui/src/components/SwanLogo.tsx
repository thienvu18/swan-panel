import React from 'react';

interface SwanLogoProps {
  className?: string;
  size?: number | string;
  showStatusDot?: boolean;
}

export const SwanLogo: React.FC<SwanLogoProps> = ({
  className = 'w-6 h-6',
  showStatusDot = true,
}) => {
  return (
    <svg
      viewBox="0 0 100 100"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      aria-label="SwanPanel Logo"
    >
      {/* Cyber Beak pointing forward (Left) */}
      <polygon points="27,26 17,29 27,32" fill="#3b82f6" />

      {/* Swan Head & Graceful S-Neck */}
      <path
        d="M 27 26 C 29 17, 44 17, 46 26 C 48 36, 39 45, 36 54 C 33 62, 35 68, 42 74 L 32 74 C 23 66, 23 53, 28 43 C 31 36, 29 31, 27 26 Z"
        className="fill-slate-900 dark:fill-slate-50 transition-colors"
      />

      {/* Modular Wing: 3 Dashboard Control Sliders */}
      {/* Slider 1: Top bar with live status dot */}
      <rect x="44" y="39" width="38" height="7" rx="3.5" fill="#2563eb" />
      {showStatusDot && (
        <circle cx="75" cy="42.5" r="2.2" fill="#10b981" />
      )}

      {/* Slider 2: Middle bar */}
      <rect x="40" y="50" width="46" height="7" rx="3.5" fill="#3b82f6" />

      {/* Slider 3: Bottom bar */}
      <rect x="35" y="61" width="41" height="7" rx="3.5" fill="#60a5fa" />
    </svg>
  );
};

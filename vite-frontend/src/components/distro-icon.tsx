import type { SVGProps } from "react";

// Map of Linux distribution names to their SVG logo components.
// Version strings from the agent look like "v1.0.0 (ubuntu/amd64)".

const distroColors: Record<string, string> = {
  ubuntu: "#E95420",
  debian: "#A81D33",
  centos: "#262577",
  rocky: "#10B981",
  alma: "#0F4266",
  alpine: "#0D597F",
  fedora: "#51A2DA",
  arch: "#1793D1",
  opensuse: "#73BA25",
  suse: "#73BA25",
  rhel: "#EE0000",
  redhat: "#EE0000",
  oracle: "#F80000",
  amazon: "#FF9900",
  amzn: "#FF9900",
  kali: "#557C94",
  mint: "#87CF3E",
  gentoo: "#54487A",
  void: "#478061",
  nixos: "#7EBAE4",
  manjaro: "#35BF5C",
};

function UbuntuIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" {...props}>
      <circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" strokeWidth="1.5" />
      <circle cx="12" cy="5" r="2" />
      <circle cx="5.5" cy="15.5" r="2" />
      <circle cx="18.5" cy="15.5" r="2" />
      <circle cx="12" cy="12" r="3" fill="none" stroke="currentColor" strokeWidth="1.5" />
    </svg>
  );
}

function DebianIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" {...props}>
      <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm3.5 14.5c-1.5 1.2-3 1.5-4.5 1.3-2-.3-3.5-1.5-4.2-3.3-.8-2-.5-4 .8-5.6 1-1.2 2.3-1.9 3.9-2 1.2 0 2 .3 2 .3l-.1 1s-.8-.4-1.7-.3c-2 .1-3.5 1.7-3.4 3.8.1 1.8 1.3 3.2 3 3.5 1.2.2 2.3-.1 3.2-.8l1 2.1z" />
    </svg>
  );
}

function CentOSIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" {...props}>
      <rect x="3" y="3" width="8" height="8" rx="1" opacity="0.9" />
      <rect x="13" y="3" width="8" height="8" rx="1" opacity="0.7" />
      <rect x="3" y="13" width="8" height="8" rx="1" opacity="0.7" />
      <rect x="13" y="13" width="8" height="8" rx="1" opacity="0.5" />
      <circle cx="12" cy="12" r="3" fill="none" stroke="currentColor" strokeWidth="1.5" />
    </svg>
  );
}

function AlpineIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" {...props}>
      <path d="M12 3L2 19h20L12 3zm0 4l6 10H6l6-10z" />
    </svg>
  );
}

function FedoraIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" {...props}>
      <circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" strokeWidth="1.5" />
      <path d="M12 6v6h6" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <text x="8" y="17" fontSize="6" fontWeight="bold" fill="currentColor">f</text>
    </svg>
  );
}

function ArchIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" {...props}>
      <path d="M12 2l-2 5c1 1.5 1.5 3 2 5 .5-2 1-3.5 2-5L12 2zM8.5 10C6.5 14 4 17 2 20c2.5-1.5 4.5-2.5 7-3-1-2-1.5-4.5-0.5-7zM15.5 10c1 2.5.5 5-.5 7 2.5.5 4.5 1.5 7 3-2-3-4.5-6-6.5-10z" />
    </svg>
  );
}

function DefaultLinuxIcon(props: SVGProps<SVGSVGElement>) {
  // Tux-inspired simple penguin outline
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" {...props}>
      <path d="M12 2C9.8 2 8 4.1 8 6.5c0 1.2.4 2.2 1 3l-1.5 4.5c-.3 1-.5 2-.5 3 0 2.2 2.2 4 5 4s5-1.8 5-4c0-1-.2-2-.5-3L15 9.5c.6-.8 1-1.8 1-3C16 4.1 14.2 2 12 2zm-1.5 6a1 1 0 110-2 1 1 0 010 2zm3 0a1 1 0 110-2 1 1 0 010 2zm-3 3h3l.5 1h-4l.5-1z" />
    </svg>
  );
}

type DistroIconProps = SVGProps<SVGSVGElement> & {
  distro: string;
};

/**
 * Detect the Linux distro from a version string like "v1.0.0 (ubuntu/amd64)"
 * and return the normalized distro key.
 */
export function parseDistroFromVersion(version?: string): string {
  if (!version) return "linux";
  const lower = version.toLowerCase();

  // Try to extract from parenthetical format: (distro/arch)
  const match = lower.match(/\(([^/)\s]+)/);
  if (match) {
    return match[1].trim();
  }

  // Fallback: check for known distro names anywhere in the string
  const knownDistros = [
    "ubuntu", "debian", "centos", "rocky", "alma", "alpine",
    "fedora", "arch", "opensuse", "suse", "rhel", "redhat",
    "oracle", "amazon", "amzn", "kali", "mint", "gentoo",
    "void", "nixos", "manjaro",
  ];
  for (const d of knownDistros) {
    if (lower.includes(d)) return d;
  }

  return "linux";
}

/**
 * Returns a branded color for the detected distro.
 */
export function getDistroColor(distro: string): string {
  const key = distro.toLowerCase();
  for (const [name, color] of Object.entries(distroColors)) {
    if (key.includes(name)) return color;
  }
  return "#6366F1"; // default indigo for unknown linux
}

/**
 * Render an SVG icon for the given Linux distribution.
 */
export function DistroIcon({ distro, ...props }: DistroIconProps) {
  const key = distro.toLowerCase();

  if (key.includes("ubuntu")) return <UbuntuIcon {...props} />;
  if (key.includes("debian")) return <DebianIcon {...props} />;
  if (key.includes("centos") || key.includes("rocky") || key.includes("alma")) return <CentOSIcon {...props} />;
  if (key.includes("alpine")) return <AlpineIcon {...props} />;
  if (key.includes("fedora")) return <FedoraIcon {...props} />;
  if (key.includes("arch") || key.includes("manjaro")) return <ArchIcon {...props} />;

  return <DefaultLinuxIcon {...props} />;
}

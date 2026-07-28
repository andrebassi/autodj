/* TS mirror of the backend /api/v1/ui/* payloads (domain/content in Go).
   Types only — the actual data comes from the API via the hooks in hooks/. */

import type { OpCategoryKey } from "@/lib/operational";

export type OpCategory = { key: OpCategoryKey; label: string; ico: string };
export type OpFolder = { name: string; ico: string; count: number };
export type CartButton = { label: string; dur: string; ico: string; hue: number };
export type Marker = { key: string; label: string; ico: string; color: string };

export type MixConfig = {
  musicaIn: number;
  musicaOut: number;
  vinhetaIn: number;
  vinhetaOut: number;
  comercialIn: number;
  comercialOut: number;
  overlapVol: number;
  talkVol: number;
  trimMusicas: boolean;
  trimVinhetas: boolean;
  normalize: boolean;
  normalizeVol: number;
  detectVocal: boolean;
  autoInstrumental: boolean;
};

export type Operational = {
  categories: OpCategory[];
  musicSubtabs: string[];
  folders: OpFolder[];
  cartwall: CartButton[];
  markers: Marker[];
  mix: MixConfig;
};

export type SubTab = { name: string; url: string; framed: boolean; brand: string };
export type WebTab = {
  name: string;
  icon: string;
  url: string;
  framed: boolean;
  subtabs?: SubTab[];
};

export type ProgramRow = { kind: string; label?: string; dur?: string; text?: string };
export type Program = { above: ProgramRow[]; below: ProgramRow[] };

/* empty defaults so components render before the fetch resolves */
export const EMPTY_OPERATIONAL: Operational = {
  categories: [],
  musicSubtabs: [],
  folders: [],
  cartwall: [],
  markers: [],
  mix: {
    musicaIn: 500,
    musicaOut: 2000,
    vinhetaIn: 50,
    vinhetaOut: 1000,
    comercialIn: 50,
    comercialOut: 1000,
    overlapVol: 30,
    talkVol: 20,
    trimMusicas: true,
    trimVinhetas: false,
    normalize: true,
    normalizeVol: 89,
    detectVocal: true,
    autoInstrumental: false,
  },
};

/* live data layer shared types (real Go API + WebSocket) */

export type Playable = {
  track_id: string;
  title: string;
  artist?: string; // intérprete/cantor — mostrado no deck, fila e histórico
  source: string;
  uri: string; // caminho de arquivo (local) OU spotify:uri — dirige o SDK no browser
  duration_ms: number;
  origin?: string; // procedência no enqueue (playlist/grade/evento) — só informativo
  artwork_url?: string; // capa direta (CDN Spotify) — render sem depender do /artwork
  category?: string; // balde de rotação ("music", "jingle"…) — o trim é por tipo

  /* Dicas de ANÁLISE (só faixa local/PCM; streaming vem tudo zero). Viajam no
     Playable, e não num fetch de biblioteca, porque o snapshot já é espelhado em
     localStorage: assim as dicas funcionam OFFLINE de graça, enquanto um fetch
     no caminho do deck não funcionaria. 0 = desconhecido / nada a fazer. */
  cue_in_ms?: number; // onde o áudio começa de fato (corta o vazio da frente)
  cue_out_ms?: number; // onde o áudio termina de fato (corta o vazio do fim)
  // BPM medido; 0 = não medido. Viaja aqui porque é ESTÁTICO — o estado do
  // SEM-VOZ não viaja, ele termina minutos depois do enfileiramento.
  bpm?: number;
  vocal_ms?: number; // onde a voz entra (0 = não marcado, de propósito)
  loudness_lufs?: number; // loudness integrado medido (0 = não medido)
};

/* Faixa já tocada + quando tocou (epoch ms) — rastreabilidade no histórico.
   Campo client-only, não vem da API; carimbado ao empurrar p/ o histórico. */
export type PlayedItem = Playable & { played_at?: number };

export type QueueItem = { playable: Playable; transition: string };

export type NowPlaying = {
  type: string;
  now_playing: Playable;
  queue: QueueItem[];
};

/* Carga da grade por hora (0..23) = nº de itens agendados naquela hora.
   O DADO VEM DA API (GET /api/v1/ui/grade-load) — nada hardcoded no front.
   Fonte hoje é mock/seed do backend; o scheduler (Épico 4) passa a derivar da
   grade real, sem mudar o contrato. */

export type GradeLoad = { hourly: number[] }; // sempre 24 posições

// enquanto a API não respondeu: 24 zeros (barras chatas, sem inventar carga)
export const EMPTY_HOURLY: number[] = Array.from({ length: 24 }, () => 0);

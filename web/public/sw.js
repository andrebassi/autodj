/* Service Worker do AutoDJ — o app abre mesmo sem servidor.

   O PROBLEMA que ele resolve: até aqui, dar F5 sem internet nem carregava a tela.
   O HTML e o JS vêm do próprio servidor Go — sem ele, o browser mostra a página de
   erro e acabou. Guardar áudio não adianta nada se a casca do app não existe.

   ESTRATÉGIA POR TIPO DE PEDIDO (cada uma escolhida pelo custo de errar):

   - **navegação (o documento)**: REDE PRIMEIRO, cache como rede de segurança. O
     HTML é servido com `no-cache` de propósito (é ele que aponta pros bundles com
     hash — ver DEV-7); servir cache primeiro faria deploy ficar invisível. Offline,
     cai no último HTML bom.
   - **`/_next/static/*`**: CACHE PRIMEIRO, sempre. O nome carrega hash de conteúdo,
     então o arquivo é imutável: se está no cache, é o certo.
   - **áudio da biblioteca**: CACHE PRIMEIRO. É o que faz a música tocar offline.
     Só respostas COMPLETAS (200) entram — pedido com `Range` é fatia (206) e não
     serve pra reconstruir o arquivo; esse passa direto pra rede.
   - **resto da API**: SEM CACHE. Fila, now-playing e biblioteca são estado vivo;
     devolver uma cópia velha faria a tela mentir sobre o que está no ar.

   Versionar o cache pelo nome é o que permite trocar de estratégia sem deixar lixo
   antigo servindo — `activate` limpa o que não é da versão corrente. */

/* Bump SHELL_REV a cada mudança de ESTRATÉGIA deste arquivo (não a cada release —
   release nova é detectada pelo diff do próprio sw.js). Serve pra forçar uma
   ativação limpa quando as regras de cache mudam. */
const SHELL_REV = 1;
const SHELL = `autodj-shell-v${SHELL_REV}`;
const AUDIO = "autodj-audio-v1"; // MESMO nome usado por lib/audioBuffer.ts

self.addEventListener("install", (e) => {
  // assume o controle já na primeira visita, sem esperar a aba fechar
  self.skipWaiting();
  e.waitUntil(caches.open(SHELL));
});

/* ATUALIZAÇÃO DE VERSÃO — como uma release nova chega sem ninguém fazer nada.

   1. O browser rebusca `sw.js` a cada navegação e compara BYTE A BYTE. Mudou →
      instala a nova. `skipWaiting` + `clients.claim` fazem ela assumir na hora, sem
      esperar todas as abas fecharem (o comportamento padrão, que prende gente em
      versão velha por dias).
   2. O DOCUMENTO é rede-primeiro: online, sempre vem o HTML do deploy novo — que é
      quem aponta pros bundles novos. Cache aqui é só rede de segurança offline.
   3. Os bundles têm HASH no nome, então "cache primeiro" nunca devolve o antigo
      pro HTML novo: a URL mudou, é outro arquivo.
   4. Ao ativar, o SHELL é APAGADO INTEIRO. Os bundles velhos ficariam guardados pra
      sempre (a URL antiga nunca mais é pedida, então nada os expulsa) e o cache
      cresceria a cada deploy. O áudio (`AUDIO`) é preservado de propósito: custa
      megabytes e download, e não tem relação com a versão do app.
   Isto é o que faz "lançar versão nova" ser um deploy comum, sem passo manual. */
self.addEventListener("activate", (e) => {
  e.waitUntil(
    (async () => {
      const names = await caches.keys();
      await Promise.all(
        names.filter((n) => n !== AUDIO).map((n) => caches.delete(n)), // shell zerado
      );
      await self.clients.claim();
    })(),
  );
});

const isStatic = (url) => url.pathname.startsWith("/_next/static/");
const isAudio = (url) =>
  url.pathname.startsWith("/api/v1/library/tracks/") && url.pathname.endsWith("/audio");

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return; // CDN do Spotify/YouTube passa direto

  // 1) documento: rede primeiro (deploy tem de aparecer), cache como rede de segurança
  if (req.mode === "navigate") {
    event.respondWith(
      (async () => {
        try {
          const fresh = await fetch(req);
          const c = await caches.open(SHELL);
          c.put("/", fresh.clone()); // guarda SEMPRE em "/" — é a única rota do app
          return fresh;
        } catch {
          const c = await caches.open(SHELL);
          return (await c.match("/")) || Response.error();
        }
      })(),
    );
    return;
  }

  // 2) bundle com hash no nome: imutável, cache primeiro
  if (isStatic(url)) {
    event.respondWith(
      (async () => {
        const c = await caches.open(SHELL);
        const hit = await c.match(req);
        if (hit) return hit;
        const fresh = await fetch(req);
        if (fresh.ok) c.put(req, fresh.clone());
        return fresh;
      })(),
    );
    return;
  }

  // 3) áudio: cache primeiro — é o que toca offline
  if (isAudio(url)) {
    if (req.headers.get("range")) return; // fatia: deixa a rede resolver
    event.respondWith(
      (async () => {
        const c = await caches.open(AUDIO);
        const hit = await c.match(req.url);
        if (hit) return hit;
        const fresh = await fetch(req);
        if (fresh.ok && fresh.status === 200) c.put(req.url, fresh.clone());
        return fresh;
      })(),
    );
    return;
  }

  // 4) resto da API: estado vivo, nunca de cache.
});

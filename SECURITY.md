# Política de segurança

## Como reportar

Encontrou uma vulnerabilidade? **Não abra issue pública.** Use o
[Security Advisory privado do GitHub](../../security/advisories/new), que permite
discutir e corrigir antes de tornar público.

Resposta esperada em alguns dias. Este é um projeto mantido por uma pessoa nas horas
livres — não há SLA, e vale a pena dizer isso em vez de prometer o que não se cumpre.

## O que vale reportar

O AutoDJ **roda na máquina de quem usa** e a versão em produção é a instância pessoal do
autor. O modelo de ameaça é modesto, mas estes casos importam:

- vazamento de credencial (Spotify, YouTube, RunPod, Fly) por log, resposta de API ou
  URL — as chaves são do usuário e a exposição é dele;
- uma estação enxergar dados de outra: o isolamento é por *fingerprint* do navegador,
  com um SQLite e um diretório de mídia por estação. Furo aí é falha de isolamento;
- escrita ou leitura fora das fronteiras do armazenamento de mídia (o `Remove` aceita
  apenas o próprio diretório e filhos diretos da raiz, justamente por isso);
- desvio da autenticação quando o servidor roda em modo multiusuário
  (`AUTODJ_SINGLE_USER=0`).

## Revisão de 2026-07 — o que foi corrigido

Uma revisão encontrou quatro pontos. Todos corrigidos; ficam registrados porque
quem audita merece saber o que já foi olhado.

| | achado | correção |
|---|---|---|
| 🔴 | A chave de API do RunPod (credencial da **conta inteira**) era injetada no pod alugado, que tem porta pública | Não vai mais. Quem encerra o pod é o watchdog do AutoDJ, que já era a autoridade — o relógio do pod virou só saída de processo |
| 🔴 | O serviço no pod não tinha autenticação e o RunPod o publica em `https://<pod-id>-8000.proxy.runpod.net`; quem achasse o endereço usava a GPU paga pelo operador | Token aleatório de 32 bytes por pod, comparado com `compare_digest`. `/health` também exige, senão vira confirmador gratuito de "há pod vivo aqui" |
| 🟡 | `Access-Control-Allow-Origin: *` — qualquer site aberto noutra aba falava com o AutoDJ local e **lia** a resposta | Liberação só para mesma origem, loopback (dev e desktop) e a lista de `AUTODJ_CORS_ORIGINS`. `X-AutoDJ-Session` entrou em `Allow-Headers` **porque** a origem passou a ser restrita |
| 🟡 | O container rodava como **root**, executando `ffmpeg`/`aubio` sobre arquivos enviados pelo usuário | Usuário `app` (uid 10001). O volume mudou de `/root/.autodj` para `/home/app/.autodj` — é o mesmo volume, e os dois têm de mudar juntos |

### Varreduras de dependência e imagem

Rodadas na mesma revisão, com o resultado registrado porque número sem contexto
assusta à toa:

| ferramenta | antes | depois |
|---|---|---|
| `govulncheck` (Go, só o que o código **alcança**) | 14 — 13 da biblioteca padrão, 1 no `golang.org/x/text` | **0**, fixando `toolchain go1.26.5` e `x/text` 0.39.0 |
| `pnpm audit` (interface) | 4 (3 altas) em `postcss` e `sharp` — ambas de build, não vão para o navegador | **0**, via `overrides` |
| `trivy` (imagem de produção) | 12 CRITICAL + 98 HIGH | **2 tinham correção** — e eram o mesmo `x/text` acima |

As outras 108 do Trivy são `affected`/`fix_deferred`/`will_not_fix` do Debian,
espalhadas por 42 pacotes que o `ffmpeg` arrasta (`libmbedcrypto7`, `perl-base`,
`libglib2.0-0`, `libaom3`, `python3.11`). Não há versão corrigida a instalar. Por
isso o portão do CI usa `--ignore-unfixed`: reprovar no que não tem conserto faz
o portão ser desligado, e aí nem o que tem conserto é pego.

⚠️ **O que isso expõe, dito sem rodeio:** a instância pública não tem login e
aceita upload, então o `ffmpeg` — o componente com mais CVEs abertas da imagem —
analisa arquivos vindos de qualquer pessoa. Rodar sem root reduz o estrago de
uma exploração; não elimina a exposição. Quem for expor a própria instância
deveria fechar o acesso (`AUTODJ_SINGLE_USER=0`) ou trocar o `ffmpeg` do Debian
por um binário estático, o que derruba a maior parte desses 42 pacotes.

Verificado e **sem problema**: nenhum `sh -c` (todo `exec.Command` passa argumentos
separados, então não há injeção de comando por nome de arquivo), nenhum SQL montado
por concatenação, upload limitado a 256 MiB, nomes de arquivo reduzidos ao componente
base (sem `../`), e o app desktop escutando em `127.0.0.1`.

## O que **não** é vulnerabilidade

- **Não há tela de login por padrão.** É decisão de produto: uso caseiro, uma pessoa,
  isolamento por navegador. Quem expõe a instância na internet aberta assume isso — o
  backend de autenticação existe e é a alavanca para fechar o acesso.
- **A busca sem login do YouTube usa a API interna.** É violação de Termos de Serviço,
  está declarada no README, e não é falha de segurança.
- **A separação de vocal na nuvem gasta dinheiro sem teto automático.** Está documentado
  no README e em `docs/pendencias.md`. É limitação conhecida, não vulnerabilidade — mas
  um jeito de *outra pessoa* fazer sua conta gastar, esse sim, reporte.

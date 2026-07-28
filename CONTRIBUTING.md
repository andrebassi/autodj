# Contribuir com o AutoDJ

Obrigado pelo interesse. Este documento é curto de propósito: são poucas regras, mas
elas não são negociáveis, porque cada uma existe por causa de um bug que já aconteceu.

## Antes de abrir um PR

```bash
nix develop        # ambiente completo (Go, Node, ffmpeg, aubio)
task cover         # domain 100%, adapters/app >= 90% — o CI reprova abaixo disso
task web:build     # bundle + gate de changelog
```

Sem Nix: Go (versão do `go.mod`), Node 22 + pnpm 10, `ffmpeg` e `aubio-tools` no PATH.

## As regras que o CI cobra

**Cobertura**: `domain/` 100%, `adapters/` e `app/` ≥ 90%. `postgres` e `cmd/*` ficam
fora do gate (são integration-tier). Não é meta simbólica: o domínio guarda invariantes
que, quebrados, colocam áudio errado no ar.

**Changelog por versão**: toda tag `vX.Y.Z` precisa de entrada no topo de
`web/app/lib/changelog.ts`, em português e voltada a quem USA. O build aborta sem ela.

**Idioma**: identificadores em **inglês** (função, variável, campo, tipo, prop, classe
CSS, nome de arquivo e de teste). Comentários, textos de interface e documentação em
**português**. A exceção são valores de contrato com o servidor (`?stem=instrumental`,
chaves como `dia`/`semana`/`mes`): renomear quebra a integração em silêncio — mantenha
e comente o porquê.

**Documentação no mesmo passo**: mexeu em funcionalidade, atualize a documentação junto.
Nunca deixe `README.md`, `docs/` ou `CLAUDE.md` descrevendo algo que o código não faz
mais.

## Por que o módulo Go chama `autodj` e não `github.com/…`

Porque isto é uma **aplicação**, não uma biblioteca: ninguém importa pacotes daqui.
O caminho local mantém os imports curtos (`autodj/domain/library`) e o módulo
compilável sem rede. O preço é que `go install github.com/andrebassi/autodj@latest`
não funciona — clone e rode `task serve`. Trocar isso significaria reescrever o
import de centenas de arquivos para ganhar um comando que este projeto não usa.

## Invariantes que não se relaxam

- **`spotify | youtube ⇒ mixable = false`**. Áudio com DRM (Spotify) e embed
  (YouTube) não têm PCM deste lado, então nunca entram no barramento de mixagem: sem
  crossfade, sem beatmatch, sem waveform. Imposto em duas camadas — domínio
  (`Track.Validate`) e banco (CHECK). Enfileirar e tocar essas fontes é normal e
  esperado; o que não pode é mixar.
- **Nunca reparentar o iframe do YouTube.** Mover um `<iframe>` na árvore DOM o
  recarrega, e isso mata o handshake do player. Reposicione por CSS/overlay.
- **Ganho no `<audio>.volume`**, não em Web Audio. O `AudioContext` nasce suspenso e
  exige gesto do usuário; rotear o playout por ele arrisca deixar a estação muda. A
  única exceção é a varredura de filtro do SEM-VOZ, que nasce de um clique.

Se um PR precisa quebrar uma destas, abra uma issue antes — provavelmente há um
caminho que não quebra.

## Testes

Nada de teste sintético que confirma a própria hipótese. Uma medição de qualidade de
áudio feita com senoides já "provou" que um filtro removia voz; em música real ele
removia 2 dB, ou seja, nada audível. Fixtures de parser vivem em `testdata/` e são
**saída real** da ferramenta, para que mudança de formato apareça como teste vermelho e
não como campo vazio na produção.

O `web/` não tem test runner: typecheck e build são o portão, e comportamento de
interface se verifica no navegador — de preferência medindo (amostrar o DOM ao longo do
tempo), não olhando.

## Bugs

`docs/known-issues.md` é indexado **por sintoma** ("some do deck", "trava em 00:00").
Bug com causa-raiz provada entra lá no mesmo passo da correção, no formato
sintoma → causa → correção → como reconhecer. É o arquivo que economiza o próximo dia
de alguém.

## Segurança

Não abra issue pública para vulnerabilidade — veja [SECURITY.md](SECURITY.md).

# Plan v2 — o plano como contrato de execução

**Status: implementado.** As oito fases da §13 estão no binário e verdes em `make check`.
O que ficou diferente do proposto, e por quê:

- **`## Paths` não valida existência de caminho.** O aviso da §3.2 exigiria uma
  severidade que `internal/finding` não tem — a forma JSON é congelada — e um plano
  nomeia por construção arquivos que ainda não existem, então a regra dispararia em
  entrada correta. Sob 0/1/2 isso vira erro, não aviso. A seção é validada como
  seção; o conteúdo não.
- **O slug do achado de ciclo é `task.dependency-cycle`, não `plan.dependency-cycle`.**
  A §8.3 e a §14 se contradiziam; vence a §14, porque a regra vive em
  `validate/tasks.go` junto das outras nove de flag e vale igualmente para o
  `tasks.md` de uma spec.
- **`plan reseal` não imprime diff contra o selo antigo** (§8.1): o selo é um hash, e o
  conteúdo antigo não é recuperável a partir dele. Imprime os dois hashes e aponta
  `git diff`, que é quem tem o diff.
- **Duas regras a mais que a §14 previa**: `plan.status-invalid` e `plan.unsealed` —
  um `status: approved` sem `checksum:` é um plano que diz estar selado por um selo
  que não existe, e toda leitura dele pularia em silêncio a verificação.
- **`_Reason_` também marca uma task *acrescentada* depois da aprovação**, não só uma
  removida. Responde à mesma pergunta: por que esta linha não é o que o plano
  aprovado dizia.
- **Orçamento da §9, medido**: `entry.md` + `rules/artifacts.md` + `rules/tasks.md`
  passaram de 175 linhas / 8539 bytes para 177 / 9248 — +2 linhas, +709 bytes. Os
  tetos que os testes já impunham (55 linhas por regra, 60 no entry) continuam
  respeitados; foi preciso comprimir prosa existente para caber.

O documento abaixo é o plano como foi aprovado.

---

## 1. Ponto de partida — o que já existe hoje

Antes da análise, os fatos verificados no código (não suposições):

| Fato | Onde |
|---|---|
| O template de plano **já não tem `## Notes`** — e diz explicitamente que a ausência é deliberada | `internal/assets/templates/artifacts/plan.md:42-48` |
| A estrutura atual é `frontmatter(autonomy, ci)` + `# Título` + `## Why` + `## Decomposition` + `## Tasks` | mesmo arquivo |
| A gramática de task é `- [ ] 1.1 (Unit) descrição — R1.2` e vive em `internal/artifact/parse.go` | `taskNumberRe`, `MethodologyRe`, `RequirementIDRe` |
| O validador de plano tem 5 regras: `kickoff-invalid`, `loop-invalid`, `item-has-two-records`, `unknown-spec`, `empty` — mais as 5 de task | `internal/validate/plan.go`, `tasks.go` |
| `map` tem 7 verbos, `patch` tem 9 | `internal/cli/map.go`, `patch.go` |
| `Block`/`map blocks`/`Search`/`map find` existem por causa de um `## Notes` de 411 linhas medido num plano real | comentários em `parse.go:93-102`, `search.go:10-27` |
| `mdscan` lê `Checked: m[2] != " "` — **qualquer** caractere que não seja espaço é "feito" | `internal/mdscan/mdscan.go:141` |
| `blockEnd` só absorve continuação **mais indentada** que o marcador da task | `parse.go:284-297` |
| `manifest.Hash` = sha256 sobre conteúdo com quebras normalizadas | `internal/manifest/manifest.go:105` |
| `assets.Version = "13"`; qualquer mudança de template obriga bump | `internal/assets/assets.go:94` |

Três consequências imediatas dessas linhas, que decidem boa parte do desenho abaixo:

1. **"Remover Notes" é quase todo trabalho de migração e de *proibição*, não de template.**
   O template já não escreve Notes. O que falta é o validador recusar seções fora do
   contrato — hoje um plano pode ter qualquer heading e passa limpo.
2. **Flags na coluna 0 não pertencem à task pelo parser atual.** No exemplo do escopo
   (`- [ ] 2.3 …` / linha em branco / `_Depends 2.1_`), `blockEnd` para na primeira
   linha com indentação ≤ 0. Sem mudança no parser, `map show 2.3` não devolve as flags,
   `patch rm 2.3` deixa as flags órfãs e `patch task 2.3 --text` apaga a continuação
   (`SetTask` zera `Detail`) mas não as flags — que ficam grudadas na task errada.
3. **`- [-]` não serve para "removed".** Seria lido como concluída. O estado `Removed`
   tem de ser uma flag, não um caractere na caixa.

---

## 2. Análise da alteração

### 2.1 O que muda de fato

O plano deixa de ser um documento híbrido (prosa + decomposição + checklist) e passa a
ser **uma tabela de execução com um cabeçalho curto**. A mudança real não é tirar uma
seção: é passar o validador de *permissivo* (aceita qualquer seção) para *fechado*
(aceita só as oito), e mover todo metadado de task para um vocabulário único de flags.

### 2.2 Onde o ganho de token realmente aparece

Vale medir antes de prometer. O ganho por chamada vem de três lugares, em ordem de
tamanho:

1. **O plano em si** — de ~56KB medidos para o teto que o contrato impõe. É o maior
   ganho e é permanente, porque a seção fechada impede o crescimento.
2. **A regra `artifacts.md`** — pré-carregada em toda requisição no Claude Code
   (`PreloadsRules`). Cuidado: o contrato novo tem **mais** conceitos a explicar
   (flags, lifecycle, discovery, selo). Se a regra crescer 40 linhas, parte do ganho
   volta para o prejuízo, em todas as requisições da sessão. §9 trata isso como
   orçamento, não como consequência.
3. **A superfície de leitura procedural** (§8.4) — troca "abrir o plano para entender o
   trabalho" por `brief` uma vez e `--next` por task. É o ganho de *releitura*: hoje o
   custo de 14k tokens é pago toda vez que a sessão perde contexto ou troca de grupo.
   Com o cabeçalho e as tasks separados por comando, nenhuma pergunta sobre o plano tem
   como resposta "abra o arquivo".

### 2.3 O que a mudança **não** resolve

- Não reduz o tamanho das specs — e as specs são ~90k tokens do corpus medido. O escopo
  diz que a spec fica inalterada; então o corpus continua dominado por specs.
- O selo (checksum) **não impede** edição direta. Ele torna a edição direta *visível*.
  Um harness que queira burlar roda `scc plan approve --force` ou calcula sha256. O
  valor é evidência e disciplina, não segurança. Está registrado aqui para que ninguém
  construa uma garantia em cima disso depois.

---

## 3. Novo contrato do Plan

### 3.1 Layout

```md
---
autonomy: auto
ci: wait
status: approved
checksum: 9f2c…            # sha256, escrito só pelo scc
---

# <Título>

<Uma a três frases: o que a feature é. Sem heading próprio.>

## Why
<Um parágrafo. Por que isto existe e o que "pronto" significa no todo.>

## Paths
- `internal/artifact/parse.go`
- `internal/cli/map.go`

## References
- specs/plan-format/
- docs/adr/0007-plan-seal.md

## Out of scope
- Reescrever o formato de spec

## Tasks
- [ ] 1.1 (Unit) Descrição imperativa
  _Depends 1.0_
  _Priority 2_

## Done when
- `scc validate` limpo e `make check` verde
```

### 3.2 Regras do contrato

| Seção | Obrigatória | Conteúdo | Regra de validação |
|---|---|---|---|
| `# Título` (H1) | sim | uma linha | `plan.missing-title` |
| descrição | sim | prosa antes do primeiro H2, ≤ N linhas | `plan.missing-description`, `plan.description-too-long` |
| `## Why` | sim | um parágrafo | `plan.missing-section` |
| `## Paths` | não | lista de caminhos | itens são lista; caminho inexistente → *aviso*, não erro |
| `## References` | não | lista de links/specs/ADRs | `plan.unknown-spec` reaproveitado |
| `## Out of scope` | não | lista | — |
| `## Tasks` | sim | só tasks e suas flags | `plan.missing-section`, `plan.empty` |
| `## Done when` | sim | lista de critérios verificáveis | `plan.missing-section` |
| qualquer outro H2 | — | **proibido** | `plan.unknown-section` (mensagem especial para `Notes`) |

Decisões embutidas, e o motivo:

- **Ordem das seções não é validada.** Presença sim, ordem não. Validar ordem custa uma
  regra e uma migração a mais e não muda nenhuma leitura — tudo é endereçado por slug.
- **Sem limite de linhas por seção, exceto na descrição.** O limite real é a seção
  fechada: sem `Notes` e sem heading livre, não há onde a prosa crescer. Um limite
  numérico em `Why` seria uma regra que dispara em plano legítimo — e o pior bug do
  produto é validador que dispara na saída correta.
- **`## Decomposition` sai do contrato.** As referências de spec passam a viver em
  `## References` (decisão **D1**, §11). O parser as reconhece por citarem
  `specs/<feature>/`, não pelo heading, então `Leaf`, o endereço `specs/foo/` e
  `map trace` seguem funcionando sem alteração.

---

## 4. Contrato da Task

### 4.1 Formato

```
- [ ] <id> (<estratégia>) <descrição>
  _Depends <id>[, <id>…]_
  _Priority <n>_
  _Status removed_
  _Reason <texto>_
```

Mapeamento para o que o escopo pediu:

| Pedido | Como | Novo? |
|---|---|---|
| identificador único | `<id>` = `<grupo>.<item>` | não — já existe, já validado |
| descrição objetiva | texto após a anotação | não |
| **estratégia de teste** | `(Unit)` \| `(TDD)` | **não** — é exatamente a anotação de metodologia existente |
| zero ou mais dependências | `_Depends_` | sim |
| zero ou uma prioridade | `_Priority_` | sim |
| zero ou mais flags | vocabulário fechado abaixo | sim |

**Reaproveitar `(Unit)`/`(TDD)` como estratégia de teste é a decisão de menor custo do
plano inteiro**: zero migração, zero mudança de parser, `patch task --method` e o
validador `task.missing-methodology` continuam valendo, e a regra `methodology.md` já
explica quando é cada uma. Inventar um `_Test_` novo duplicaria o conceito.

### 4.2 Vocabulário de flags — fechado

| Flag | Valor | Cardinalidade | Semântica |
|---|---|---|---|
| `_Depends_` | lista de ids separados por vírgula | 0..1 linha | a task só é elegível quando **todas** estiverem `[x]` |
| `_Priority_` | inteiro ≥ 1 | 0..1 | menor = mais urgente; ausente = menos urgente que qualquer explícita |
| `_Status_` | **apenas** `removed` | 0..1 | remoção lógica por Discovery |
| `_Reason_` | texto livre de uma linha | 0..1 | obrigatório com `_Status removed_` |

Três decisões aqui, e as três são para evitar bug conhecido:

- **`_Status` não aceita `open`/`completed`.** A caixa é o estado. Aceitar os dois
  criaria dois registros de um fato — exatamente o defeito que
  `plan.item-has-two-records` existe para pegar. Regra: `task.status-duplicates-box`.
- **Vocabulário fechado.** Um `_Qualquer coisa_` em itálico logo abaixo de uma task
  **não** é absorvido como flag: vira `task.unknown-flag`. Sem isso, prosa em itálico
  seria engolida silenciosamente pela task anterior — o modo de falha do §1.2.
- **Nada de `_Blocked_`.** É derivável de `_Depends_`. Duas fontes para um fato
  divergem; a derivada ganha.

Avaliadas e **rejeitadas** (registrado para não voltarem sem argumento novo):
`_Owner_` (o dono é o git), `_Estimate_` (não é verificável, e o produto só valida o
que é verificável), `_Tags_` (é busca, e busca é `map`), `_Parallel_` — já existe
decisão registrada em `rules/tasks.md:20` de que não há marcador de paralelismo.
`_Spec_` foi rejeitada com **D1**: uma task que aponta para uma spec teria a caixa e o
`tasks.md` daquela spec como dois registros de um mesmo estado. A referência fica em
`## References` e a task que a consome é marcada quando a spec fecha.

### 4.3 Parsing — o ponto técnico que decide o resto

Uma flag é uma linha que casa `^\s*_(Depends|Priority|Status|Reason)\b(.*)_\s*$`,
imediatamente após o bloco da task (continuação incluída), tolerando **no máximo uma**
linha em branco antes do bloco de flags e nenhuma dentro dele. O bloco de flags para na
primeira linha que não é flag.

Consequências que **precisam** ser implementadas juntas, ou o resultado é pior que hoje:

1. `Task.End` passa a cobrir as flags → `map show 1.1` devolve task + flags,
   `patch rm 1.1` remove as duas coisas, o rollback funciona.
2. `Task.Detail` **exclui** as linhas de flag → a busca não indexa `_Priority 2_` como
   texto, e `--width` continua honesto.
3. `renderTask` reemite as flags em ordem canônica (`Depends`, `Priority`, `Status`,
   `Reason`) → `patch task --text` deixa de destruir metadado. **Hoje isso seria um bug
   de perda de dados**: `SetTask` zera `Detail` e re-renderiza só o cabeçalho.
4. Indentação canônica das flags = 2 espaços. O exemplo do escopo usa coluna 0; aceitar
   os dois na leitura e normalizar para 2 na escrita é o meio-termo: coluna 0 sobrevive
   se alguém escrever à mão, mas o que o scc escreve é sempre indentado — e indentado
   já pertence ao bloco pelas regras que o resto do produto usa.

Tudo isso vive em `internal/artifact`, que já é o dono declarado das gramáticas
(`CLAUDE.md`: "`internal/artifact` owns the grammars, and `internal/validate` consumes
them"). O parser afirma fatos; transformar fato em finding continua em `validate` —
que é o que mantém `map` legível sobre arquivo malformado.

### 4.4 Serialização

- Escrita canônica: cabeçalho quebrado em `LineWidth = 88` (já existe), flags uma por
  linha, ordem fixa, 2 espaços de indentação, sem linha em branco entre task e flags.
- **Idempotência é requisito de teste**: `parse(render(parse(x))) == parse(x)` para todo
  plano do corpus. É o que garante que `patch` duas vezes não produz diff na segunda.
- JSON: `Task` ganha `depends []string`, `priority *int`, `status string`,
  `reason string`, `blocked bool` (derivado), `eligible bool` (derivado). Campos
  derivados entram no JSON porque o consumidor é um agente que não deve recalcular
  regra de elegibilidade — se recalcular, temos duas implementações do `--next`.

---

## 5. Lifecycle

Três estados, como pedido. **Nenhum outro** — avaliei e rejeito os candidatos:

| Estado | Representação | Quem escreve |
|---|---|---|
| `Open` | `- [ ]` sem `_Status_` | autor (draft) ou Discovery |
| `Completed` | `- [x]` | `scc patch check` |
| `Removed` | `- [ ]` + `_Status removed_` + `_Reason_` | `scc patch rm` em plano aprovado |

Rejeitados e por quê:
- **`InProgress`** — o progresso dentro de uma task é o todo-list do harness, e
  `rules/tasks.md:39-49` já decidiu que o arquivo guarda o durável e o harness guarda o
  agora. Um terceiro estado teria de ser escrito e limpo por sessão que pode morrer no
  meio, deixando lixo que ninguém reconcilia.
- **`Blocked`** — derivado de `_Depends_`.
- **`Skipped`** — é `Removed` com outra `_Reason_`.

Invariantes:
- `Removed` + `[x]` → `task.removed-but-checked`.
- Task `Removed` nunca conta em `Done()`, nunca aparece em `--next`, **conta** para a
  alocação de número (§7).
- Dependência apontando para task `Removed` → `task.depends-on-removed` (achado, não
  aviso: é um deadlock silencioso em `--next`).

---

## 6. Discovery

**Discovery é um modo de escrita, não um comando novo.** Reutiliza `patch add` / `patch
rm`, que já resolvem endereço, revalidam e fazem rollback. Um par de verbos novos
(`scc plan discover add`) seria uma segunda superfície fazendo a mesma coisa.

Em plano com `status: approved`:

| Operação | Comportamento |
|---|---|
| `patch add` | exige `--reason`; **recusa `--number`** (o scc aloca, §7); exige `--group N` ou `--new-group` |
| `patch rm` | **remoção lógica**: reescreve a task com `_Status removed_` + `_Reason`; nunca apaga linhas |
| `patch check`/`uncheck` | permitido |
| `patch fm` | permitido (respostas do loop: `pr`, `worktree`, `merge`) |
| `patch task --text/--method/--number` | **recusado** — muda conteúdo funcional |
| `patch append`/`prepend`/`replace` | **recusado em plano aprovado** |

Em plano `status: draft`: tudo permitido, `rm` apaga de verdade, `add` aceita
`--number`. É a fase de autoria.

O que Discovery **nunca** pode tocar está garantido estruturalmente, não por instrução:
`Why`, `Out of scope`, `Done when` e o título só são alcançáveis por
`append`/`prepend`/`replace`, e esses três estão recusados em plano aprovado. Não
existe caminho por `add`/`rm`/`check`, que só endereçam tasks.

Histórico preservado em duas camadas: a task removida **fica no arquivo** com sua razão,
e o git guarda o resto. Não há terceiro log — um log que ninguém mantém contradiz os
outros dois.

---

## 7. Numeração

- Formato `<grupo>.<item>`, ambos inteiros ≥ 1.
- Grupos sequenciais a partir de 1; itens sequenciais dentro do grupo.
- **Imutável**: `patch task --number` recusado em plano aprovado.
- **Nunca reutilizado**: a próxima vaga é `max(item) + 1` no grupo, contando tasks
  `Removed`. Como a removida permanece no arquivo, o high-water mark é derivável — **sem
  estado extra em lugar nenhum**, o que preserva a convenção "um arquivo por harness e
  nenhum arquivo de configuração".
- `--new-group` aloca `max(grupo) + 1`.
- Regra nova `plan.number-gap`? **Não.** Buraco de numeração é consequência normal de
  remoção; validar geraria achado em plano correto.

---

## 8. SCC — impacto comando a comando

### 8.1 Novos

| Comando | O que faz | Saída |
|---|---|---|
| `scc plan approve <name>` | valida; se limpo, escreve `status: approved` + `checksum`; se sujo, recusa | 0 / 2 |
| `scc plan reseal <name>` | recalcula o selo após edição legítima fora do ciclo (ex.: resolução de conflito de merge). Exige `--force` e **imprime o diff contra o selo antigo** | 0 / 1 |
| `scc plan migrate <name>` | converte plano v1 → v2 (§10) | 0 / 1 |
| `scc map brief <artifact>` | o cabeçalho do plano sem as tasks — o "o que é este trabalho" (§8.4) | 0 |
| `scc map tasks … --ready` | todas as tasks elegíveis, na ordem canônica | 0 |
| `scc map tasks … --blocked` | abertas não elegíveis, cada uma nomeando o que espera | 0 |
| `scc map tasks … --deps` | só as arestas de dependência, uma linha por task | 0 |
| `scc map tasks … --next` | passa a ter algoritmo determinístico (§8.3) | 0 |

Sem comando `plan status`: `map <plan>` já responde.

### 8.2 Removidos / depreciados

| Comando | Proposta | Justificativa |
|---|---|---|
| `scc map find` | **manter o código; tirar da documentação de plano** (**D3**, fechada) | O motivo declarado do `find` é o corpus inteiro (94 artefatos, 352KB), não o plano. Com o plano pequeno, buscar *no plano* deixa de fazer sentido — mas buscar em `design.md` e no knowledge base continua sendo a única alternativa a ler arquivo. Remover o binário não economiza token; remover a linha da regra economiza, em toda requisição. |
| `scc map blocks` | manter, escopo reduzido a specs/docs | Existia pelo `## Notes` de 411 linhas. Sem Notes, um plano não tem parágrafo endereçável — mas `design.md` tem. |
| `patch append/prepend/replace` | manter para specs/docs; **recusar em plano aprovado** | São a única forma de escrever prosa em `design.md`. Remover quebraria a autoria de spec, que o escopo diz não mudar. |
| `patch task --number` | recusar em plano aprovado | numeração imutável |
| `map tasks --method` | manter | filtro por estratégia de teste continua útil |

**Nenhum comando é removido do binário nesta proposta.** Toda a redução de superfície
acontece na documentação e nas guardas por estado. Isso é deliberado: o custo de um
subcomando não documentado é ~0 token; o custo de quebrar `scc patch replace` para quem
escreve spec é alto e imediato.

### 8.3 `--next` — algoritmo determinístico

```
entrada: um artefato (plano), suas tasks
1. candidatos = tasks com  !Checked  ∧  status ≠ removed
2. elegíveis  = candidatos cujas deps existem e estão todas Checked
3. ordenar elegíveis por:
     a) priority ascendente   (ausente = +∞, ou seja, por último)
     b) id natural            (grupo asc, depois item asc — comparação numérica, não textual)
     c) ordem de arquivo      (desempate total; na prática igual a (b))
4. devolver o primeiro
```

Casos de borda, todos com resposta definida:

| Situação | Resposta | Exit |
|---|---|---|
| há elegível | a task, `--json` com `depends`, `priority`, endereço | 0 |
| há abertas, nenhuma elegível | `{"task":null,"blocked":[{id, waiting_on:[…]}]}` + texto nomeando os bloqueadores | 0 |
| nenhuma aberta | `{"task":null,"done":true}` | 0 |
| ciclo de dependência | achado `plan.dependency-cycle`, nomeando o ciclo | 2 |
| dependência inexistente/removida | achado; a task é tratada como não-elegível | 2 |
| plano com drift de selo | achado `plan.drift`, **nada é devolvido** | 2 |

`--next` com vários artefatos mantém o comportamento atual (primeiro artefato com
elegível, na ordem do scan). Priorizar entre planos diferentes seria inventar uma
ordenação global que ninguém pediu.

Comparação numérica de id é uma correção necessária: hoje a ordem é a do arquivo, e
`1.10` < `1.9` em ordenação textual.

### 8.4 A superfície de leitura procedural

O contrato acima só vale se **existir uma consulta para cada pergunta que hoje leva a
abrir o arquivo**. Proibir a leitura direta sem oferecer a consulta equivalente produz
um agente que desobedece a regra — corretamente, porque a alternativa era não trabalhar.

O modelo é um só, e é o que torna a garantia verificável:

> **O plano é cabeçalho + tasks. `brief` lê o cabeçalho, `tasks` lê as tasks, e nenhum
> comando devolve os dois.** Não existe chamada de scc que retorne o plano inteiro.

| Pergunta do agente | Comando | O que volta | Frequência | Custo estimado |
|---|---|---|---|---|
| O que é este trabalho, e quando está pronto? | `scc map brief <plan>` | título, descrição, `Why`, `Paths`, `References`, `Out of scope`, `Done when` — **sem tasks** | **1× por sessão** | ~500–800 tokens |
| O que eu faço agora? | `scc map tasks <plan> --next` | uma task: id, estratégia, descrição, deps, prioridade, endereço | 1× por task | ~100 tokens |
| Qual é a frente de trabalho? | `scc map tasks <plan> --ready` | todas as elegíveis, na ordem de §8.3 | eventual | ~30 tokens/task |
| Por que não há nada elegível? | `scc map tasks <plan> --blocked` | abertas não elegíveis, cada uma nomeando o que espera | em impasse | ~30 tokens/task |
| Qual é a ordem geral? | `scc map tasks <plan> --deps` | só as arestas: `1.3 ← 1.1, 1.2` | eventual | ~1 linha/task |
| Me mostra exatamente essa task | `scc map show <plan> 1.2` | a task com suas flags e continuação | sob demanda | ~80 tokens |
| Quanto falta? | `scc map <plan>` | contagens por seção e por grupo: feitas / abertas / bloqueadas / removidas | por grupo | ~20 linhas |

Custo total de uma sessão que executa N tasks: **`brief` uma vez + N × `--next`**. Para
um plano de 30 tasks isso é ~800 + 3.000 tokens, contra 14k **por releitura**.

Decisões embutidas:

- **`brief` é um verbo novo em `map`, não em `plan`.** `map` já é a metade de leitura
  declarada do produto, e `brief` serve igualmente a uma spec (`Why` + contagem de
  requisitos) — pôr em `plan` daria dois lugares para ler artefato.
- **`--ready`, `--blocked` e `--deps` são flags de `map tasks`, não verbos.** A pergunta
  é a mesma ("quais tasks"), muda o filtro. Verbo novo por filtro infla o `--help`, que
  é superfície documentada e portanto custa token.
- **Todos aceitam `--json`** por `addJSON`/`emitJSON`, como manda a convenção. O JSON é
  o contrato para o harness; o texto é para a pessoa.
- **`brief` é limitado por construção, não por truncamento.** O cabeçalho é limitado
  pelo contrato de seções fechadas (§3). Um `brief` que precisasse truncar seria sinal
  de que o validador falhou, não de que o comando precisa de `--width`.
- **Nenhum desses comandos imprime prosa de task por padrão.** A descrição vai clipada
  em `--width` (já existe); a continuação inteira só sai por `map show`, que é a
  pergunta explícita "me mostra essa".

Isto fecha a exigência do escopo: com `brief` + `tasks` + `show`, não sobra pergunta
sobre o plano cuja única resposta seja abrir o arquivo — e é isso que dá autoridade à
regra "nunca leia o plano" em `rules/artifacts.md`.

### 8.5 O selo (checksum)

**Canonicalização** — o que entra no hash:
- o arquivo inteiro, **menos a linha `checksum:`** (senão o hash referencia a si mesmo),
- com quebras normalizadas para LF (`textutil.NormalizeNewlines`, já usado),
- sha256 hex — reaproveitando `manifest.Hash`, que já faz as duas coisas.

**Onde é verificado**: em *todo* comando que lê ou escreve tasks de um plano com
`status: approved` — `map tasks`, `map show`, `map <plan>`, `map index`, todos os
`patch`. Custo real ≈ 0: o arquivo já é lido; sobra um sha256 sobre ~4KB.

**Ordem de operações no `patch` — isto é load-bearing**:
```
1. carregar          5. revalidar
2. VERIFICAR selo    6. rollback se introduziu achado
3. aplicar edição    7. RESELAR (recalcular e escrever checksum)
4. escrever          8. reportar
```
Verificar **antes** de aplicar é o que impede o cenário em que o harness edita à mão,
roda `patch check` e o resela por cima, apagando a evidência.

**Erro de drift** — precisa ser acionável, porque o leitor é um agente que não pode ver
o arquivo:
```
✗ plans/x.md — drift: o arquivo mudou fora do scc
  selo:   9f2c…  (status: approved)
  atual:  41ab…
  o conteúdo funcional de um plano aprovado só muda por scc patch.
  → reverta com git, ou `scc plan reseal x --force` se a edição foi intencional
```

**Limites, ditos aqui e não descobertos depois:**
- É evidência, não impedimento. `reseal --force` está a um comando de distância e o
  sha256 é público. O valor é que a violação vira ruidosa e auditável.
- **Conflito de merge**: cada tick reescreve a linha `checksum:` → dois branches que
  marcam tasks diferentes conflitam **sempre** nessa linha. Sob `pr: per-group` com
  worktrees paralelos isso é atrito real. Mitigações consideradas: selo fora do arquivo
  (rejeitado — cria arquivo por plano, contra a convenção "nenhum arquivo de
  configuração"); selo só sobre `## Tasks` sem os estados (rejeitado — deixaria a
  alteração de `Why` passar batida, que é justamente o que se quer pegar). **Aceito o
  conflito**, com `scc plan reseal` documentado como a resolução. O `plan-run` é
  sequencial por desenho, então o caso é raro.
- Plano sem `status`/`checksum` = não selado = nenhuma verificação. É o que dá
  compatibilidade retroativa de graça.

---

## 9. Impacto em templates, regras e skills

| Arquivo | Mudança | Risco |
|---|---|---|
| `artifacts/plan.md` | reescrito no contrato v2 | `TestFreshArtifactsPassTheirOwnValidators` tem de passar — o pior bug do produto é validador que dispara na própria saída |
| `rules/artifacts.md` | tabela de perguntas, endereços, e **"nunca leia o plano"** | **orçamento**: o arquivo é pré-carregado em toda requisição no Claude Code |
| `rules/tasks.md` | gramática de flags + lifecycle | mesmo orçamento |
| `rules/autonomy.md` | menciona `status:`/`checksum:` no frontmatter | pequena |
| `skills/plan-run/SKILL.md` | trocar "mapeie e escolha o grupo" por laço sobre `--next --json`; remover a permissão de ler o plano ("Read the plan itself only where the map is not enough" → nunca); **grupo passa a ser só a família de número maior** — a tabela de duas linhas em `SKILL.md:31-35` vira uma | média — a skill é longa e tem duas formas de PR, mas **D1 a encurta** |
| `commands/scc-plan-run.md` | idem | pequena |
| `entry.md` | linhas de `map`/`patch` | pré-carregado — cada linha conta |
| `agents/code-review.md` | já usa `map show`; conferir | pequena |

**Orçamento explícito, porque é onde a meta pode ser perdida**: o conjunto
`entry.md` + `rules/artifacts.md` + `rules/tasks.md` **não pode crescer**. Medir antes e
depois (linhas e bytes) e registrar no PR. Se o contrato novo não couber, a saída é
mover a explicação de flags para dentro do `--help` do `scc patch` (que só é lido quando
consultado) e deixar na regra apenas a tabela pergunta→comando.

`assets.Version` vai para `"14"`. `scc update` já trata isso com replace-or-keep; planos
são artefatos do usuário e não são tocados por `update` — a migração é o §10.

---

## 10. Compatibilidade e migração

### 10.1 Classificação das quebras

| Mudança | Tipo | Quem sente |
|---|---|---|
| Seções fechadas | **breaking** para planos v1 com `Notes`/`Decomposition` | `scc validate` → exit 2 |
| Flags no parser | aditivo | ninguém: plano sem flag continua válido |
| `_Status removed_` | aditivo | — |
| `--next` com deps/prioridade | **comportamental** | sem flags o resultado é idêntico ao de hoje (primeira aberta) |
| Selo | aditivo e opt-in | plano sem `status:` não é verificado |
| Guardas de escrita em plano aprovado | breaking **por opção** — só depois de `approve` | — |
| Contrato de spec | **inalterado** | — |

Ordem numérica correta em `--next` (`1.9` antes de `1.10`) é a única alteração de
comportamento que aparece sem o usuário pedir. É correção de defeito; vai no changelog.

### 10.2 `scc plan migrate <name>`

Automática no mecânico, **nunca destrutiva** no conteúdo:

1. `--dry-run` por padrão? Não — `--dry-run` disponível, execução normal escreve, mas
   nada é apagado (ver 3).
2. Garante as seções obrigatórias, criando as vazias com um marcador `<!-- TODO -->`
   que o validador aceita? **Não**: cria a seção vazia e deixa o achado aparecer.
   Um placeholder que passa no validador é um plano que mente.
3. Seções fora do contrato (`Notes`, e quaisquer outras) são **movidas** para
   `plans/archive/<name>-notes.md`. `plans/archive/` é seguro: o scanner de planos usa
   `ReadDir` e ignora diretórios, então o arquivo não vira um plano fantasma.
4. Normaliza a indentação das flags e reescreve as tasks na forma canônica.
5. Escreve `status: draft`. **Nunca aprova automaticamente** — aprovar é ato humano.
6. Relatório: seções movidas, tasks reescritas, achados restantes.

Migração automática no momento da leitura foi considerada e rejeitada: reescrever
arquivo do usuário como efeito colateral de um `map` viola "never author what the user
owns" e produz diff que ninguém pediu.

### 10.3 Caminho para o workspace já existente

O plano de 56KB medido é o caso real. Com **D1** fechada, `migrate` faz três coisas
nele: move `## Notes` para `plans/archive/`, **renomeia `## Decomposition` para
`## References`** — o conteúdo é o mesmo, são itens de lista citando `specs/<feature>/`,
então nenhuma linha muda — e cria vazias as seções obrigatórias que faltam
(`Out of scope`, `Done when`), deixando os achados aparecerem em vez de preenchê-las com
placeholder. As 31 referências de spec sobrevivem intactas e `map trace` continua
respondendo sobre elas.

---

## 11. Decisões — fechadas

**D1 — `## Decomposition` sai; as referências de spec vão para `## References`.**
Decidido. O contrato fica com as oito seções e nada mais.

A consequência é mais barata do que parecia, e vale registrar por quê: `parseLeaves`
reconhece um item de lista que cita `specs/<feature>/` **em qualquer lugar do arquivo**,
não por estar sob um heading específico. Então o tipo `Leaf`, o `TargetLeaf`, o endereço
`specs/foo/` e o `map trace` continuam funcionando **sem alteração de código** — passam
apenas a encontrar seus itens sob `## References`. O que fecha a porta para eles
aparecerem em outro lugar é a própria regra `plan.unknown-section`.

O que de fato muda:
- `plan.unknown-spec` continua valendo, agora sobre `## References`.
- `plan.item-has-two-records` continua valendo e continua necessária: uma task não pode
  citar uma spec.
- **`plan-run` perde "um grupo = um leaf".** Grupo passa a ser só a família de número
  maior (`1.1`, `1.2` → grupo 1). Isso *simplifica* a skill: a tabela de duas linhas em
  `SKILL.md:31-35` vira uma. Um trabalho grande o bastante para ser uma spec vira uma
  task que aponta para a spec em `## References` e é marcada quando a spec fecha.
- O rótulo "decomposition" some do vocabulário: `map outline` deixa de imprimir
  `N leaves` como categoria própria de plano.

**D2 — O selo é verificado na leitura e na escrita.** Segue o pedido literal. `--no-verify`
existe para diagnóstico. Custo real ≈ 0: o arquivo já é lido; sobra um sha256 sobre ~4KB.

**D3 — `map find` sai da documentação, não do binário.** Decidido. `runMapFind` e
`internal/artifact/search.go` ficam; a linha some de `rules/artifacts.md` e de
`entry.md`, que é onde o token é pago em toda requisição. Continua sendo a única
alternativa a ler `design.md` inteiro.

**D4 — Flags indentadas com 2 espaços.** Decidido. A leitura aceita coluna 0 (um humano
que escreveu à mão não é punido); a escrita normaliza sempre para 2. Assim o bloco de
flags pertence à task pelas regras de indentação que o produto já usa, e `blockEnd` não
precisa de regra nova.

**D5 — `migrate` move as seções fora do contrato para `plans/archive/<name>-notes.md`.**
Decidido. Nada é apagado e nada exige `--force`. É seguro porque o scanner de planos usa
`ReadDir` e ignora diretórios: o arquivo arquivado não vira um plano fantasma nem é
validado.

---

## 12. Riscos

| # | Risco | Prob. | Impacto | Mitigação |
|---|---|---|---|---|
| R1 | `patch task --text` destrói flags | alta se não tratado | perda de dados | `renderTask` reemite flags; teste de round-trip é P0 |
| R2 | Flag parser engole prosa em itálico | média | task com região errada | vocabulário fechado; desconhecido vira achado, não conteúdo |
| R3 | Regras crescem e comem o ganho de token | **alta** | meta principal perdida | orçamento medido antes/depois no PR (§9) |
| R4 | Conflito de merge na linha `checksum:` | média | atrito em worktree paralelo | `plan reseal` documentado; `plan-run` é sequencial |
| R5 | Selo lido como garantia de segurança | média | decisão errada no futuro | registrado como tamper-*evidence* aqui e na regra |
| R6 | Validador dispara no template novo | média | pior bug do produto | `TestFreshArtifactsPassTheirOwnValidators` é gate obrigatório |
| R7 | Deadlock de `--next` por dep em task removida | média | loop trava sem explicação | achado `task.depends-on-removed` + saída `blocked` explícita |
| R8 | `plan-run` continua lendo o plano por hábito | alta | meta de contexto perdida | a superfície de §8.4 tem de existir **antes** da regra proibir a leitura — proibir sem oferecer a consulta produz desobediência justificada. Fase 3 antes da fase 7 |
| R9 | ~~D1 quebra `map trace`~~ — fechada: o parser reconhece o leaf pela citação, não pelo heading | baixa | — | resta só reescrever a noção de grupo do `plan-run` (fase 7) |
| R10 | CRLF/Windows muda o hash | baixa | drift falso | `manifest.Hash` já normaliza; teste específico no CI do Windows |

---

## 13. Estratégia de implementação

Oito fases, ordenadas para que **nada quebre antes de existir substituto**. Cada fase é
um PR com `make check` verde.

| Fase | Conteúdo | Quebra algo? |
|---|---|---|
| **0** | ~~Decidir D1–D5~~ — **fechada**, §11 | — |
| **1** | `internal/artifact`: parse de flags, `Task.End` cobrindo flags, `Detail` sem flags, `renderTask` canônico, ordenação numérica de id. Testes de round-trip | não — aditivo |
| **2** | `internal/validate/plan.go`: seções fechadas, flags, deps, ciclo, lifecycle. Template v2 + `assets.Version = "14"` | sim — planos v1 passam a ter achados |
| **3** | A superfície de leitura: `map brief`, `--next` determinístico, `--ready`, `--blocked`, `--deps`, flags no JSON | comportamental |
| **4** | Selo: `plan approve`, `plan reseal`, verificação em leitura e escrita | aditivo (opt-in) |
| **5** | Discovery: guardas por `status`, `patch rm` lógico, alocação de número | breaking só em plano aprovado |
| **6** | `scc plan migrate` | aditivo |
| **7** | Regras, `entry.md`, `plan-run`, comandos — **com medição de orçamento** | — |
| **8** | Depreciações: tirar `find` da doc de plano, ajustar `--help`, changelog | — |

Com D1–D5 fechadas, a fase 1 pode começar assim que o plano for aprovado.

---

## 14. ToDos por prioridade

### P0 — bloqueantes (nada começa sem)

1. ~~Fechar D1–D5~~ — feito, §11.
2. Definir a canonicalização exata do hash e congelá-la em teste (`golden`), incluindo
   caso CRLF.
3. Escrever `TestPlanV2TemplatePassesItsOwnValidator` **antes** do template.
4. Medir e registrar a linha de base do orçamento: linhas e bytes de `entry.md`,
   `rules/artifacts.md`, `rules/tasks.md` **hoje** — o número contra o qual a fase 7 é
   julgada.

### P1 — o núcleo

4. `artifact`: `Flag`, `Task.Depends/Priority/Status/Reason`, parse do bloco de flags.
5. `artifact`: `Task.End` inclui flags; `Detail` exclui; teste de que `map show` devolve
   task+flags.
6. `artifact`: `renderTask` reemite flags em ordem canônica — **fecha R1**.
7. `artifact`: comparação numérica de id (`1.9` < `1.10`), com teste.
8. `artifact`: `Eligible()`/`Blocked()` + detecção de ciclo.
9. `validate/plan.go`: `unknown-section` (mensagem especial p/ `Notes`), `missing-section`,
   `missing-description`.
10. `validate/tasks.go`: `unknown-flag`, `invalid-priority`, `invalid-status`,
    `status-duplicates-box`, `removed-without-reason`, `removed-but-checked`,
    `unknown-dependency`, `self-dependency`, `depends-on-removed`, `dependency-cycle`.
11. Template `artifacts/plan.md` v2 + `assets.Version = "14"`.
12. `map tasks --next`: algoritmo do §8.3, com tabela de testes cobrindo os 6 casos de borda.
13. `map tasks --json`: expor `depends`, `priority`, `status`, `blocked`, `eligible`.
14. `scc map brief <artifact>` — o cabeçalho sem as tasks (§8.4), com `--json`.
15. `map tasks --ready` / `--blocked` / `--deps`, compartilhando a ordenação de §8.3
    com `--next` — **uma implementação só**, ou teremos duas noções de elegibilidade.
16. `map <plan>`: contagens passam a distinguir feitas / abertas / **bloqueadas** /
    **removidas**.
17. Teste de orçamento: `map brief` sobre o plano de 56KB migrado tem de caber no teto
    declarado em §8.4 — se não couber, é o validador de seções que está frouxo.

### P2 — imutabilidade e discovery

18. `internal/artifact` (ou `internal/seal`): canonicalizar + hash + verificar.
19. `scc plan approve` — valida, exige limpo, sela.
20. `scc plan reseal --force` — com diff contra o selo antigo.
21. Verificação de drift nos caminhos de leitura e escrita, **antes** de aplicar (§8.5).
22. Guardas por `status: approved` em `patch task/append/prepend/replace`.
23. `patch rm` lógico + `patch add` com `--reason`, `--group`/`--new-group` e alocação
    de número por high-water mark.
24. `Done()` e `map index` ignorando tasks `Removed`.

### P3 — migração, documentação, limpeza

25. `scc plan migrate` (§10.2) — incluindo a renomeação `## Decomposition` → `## References`
    — + testes com plano v1 real como fixture.
26. Reescrever `rules/artifacts.md` e `rules/tasks.md` **medindo linhas antes/depois**;
    a tabela pergunta→comando passa a ser a de §8.4.
27. Reescrever `plan-run` para o laço `brief` (1×) + `--next` (N×) e remover a permissão
    de ler o plano.
28. `entry.md`, `commands/scc-plan-run.md`, `agents/code-review.md`.
29. Tirar `map find` da documentação de plano; ajustar `mapUsage()`/`patchUsage()`.
30. Changelog: quebras, `plan migrate`, ordenação numérica corrigida.
31. Reavaliar `map blocks` e `Search` sobre o corpus pós-migração — decidir com número,
    não com impressão.

---

## 15. Critério de pronto deste plano

- ✅ D1–D5 respondidas (§11).
- ✅ Existe uma consulta para cada pergunta que hoje leva a abrir o plano (§8.4) — sem
  isso a regra "nunca leia o plano" não teria autoridade.
- ✅ Orçamento de tokens das regras definido como número, não como intenção (P0 #4):
  55 linhas por regra e 60 no entry, impostos por `TestRulesStayShortEnoughToBePreloaded`
  e `TestScaffoldedEntryFileStaysShort`. Medição antes/depois no topo deste arquivo.
- ✅ Aprovação do plano, e implementação — ver o topo deste arquivo.

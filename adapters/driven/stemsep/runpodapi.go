package stemsep

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

/*
Cliente mínimo da API do RunPod — só o que a separação na nuvem precisa.

Adaptado de `finetuning-platform/internal/backend/runpod.go`, e deliberadamente
COPIADO em vez de importado: são módulos Go independentes, evoluem em ritmos
diferentes, e acoplá-los por dependência criaria trabalho toda vez que um mudasse.
O que veio de lá é o conhecimento (endpoint, forma das mutations, preflight de
saldo), não a dependência.

⚠️ `podTerminate` responde `{"data":{"podTerminate":null}}` MESMO QUANDO FUNCIONA
— medido. Logo `Terminate` não pode ser usado como confirmação: quem precisa saber
se o pod morreu tem de CONSULTAR o estado depois (ver o watchdog em runpod.go).
*/
// var, não const: os testes apontam o cliente para um servidor local. Bater na API
// de verdade num teste unitário custaria dinheiro e dependeria de rede.
var runpodEndpoint = "https://api.runpod.io/graphql"

// ErrNoCapacity indica que o RunPod não tem a GPU pedida disponível agora. Não é
// falha do nosso lado: capacidade de nuvem some e volta, e quem chama deve tentar
// outro modelo (ver a lista de fallback).
var ErrNoCapacity = errors.New("runpod: sem capacidade para esta GPU")

type runpodClient struct {
	apiKey string
	http   *http.Client
}

func newRunpodClient(key string) *runpodClient {
	return &runpodClient{apiKey: key, http: &http.Client{Timeout: 60 * time.Second}}
}

type podInfo struct {
	ID            string  `json:"id"`
	DesiredStatus string  `json:"desiredStatus"`
	CostPerHr     float64 `json:"costPerHr"`
	Runtime       *struct {
		UptimeSeconds int `json:"uptimeInSeconds"`
		Ports         []struct {
			PrivatePort int    `json:"privatePort"`
			PublicPort  int    `json:"publicPort"`
			Type        string `json:"type"`
		} `json:"ports"`
	} `json:"runtime"`
}

func (c *runpodClient) do(ctx context.Context, query string, vars map[string]any, out any) error {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, runpodEndpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("runpod: resposta ilegível: %s", truncate(string(body), 200))
	}
	if len(env.Errors) > 0 {
		msg := env.Errors[0].Message
		// A mensagem de capacidade é a única que quem chama trata de forma
		// diferente (tenta outra GPU), então vira sentinela em vez de texto.
		if strings.Contains(strings.ToLower(msg), "no longer any instances") {
			return fmt.Errorf("%w: %s", ErrNoCapacity, truncate(msg, 120))
		}
		return fmt.Errorf("runpod: %s", truncate(msg, 200))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// balance devolve o saldo em USD. É o preflight: lançar pod sem saldo falha de um
// jeito confuso (o pod some sozinho), e conferir antes custa uma chamada.
func (c *runpodClient) balance(ctx context.Context) (float64, error) {
	var out struct {
		Myself struct {
			ClientBalance float64 `json:"clientBalance"`
		} `json:"myself"`
	}
	if err := c.do(ctx, `query { myself { clientBalance } }`, nil, &out); err != nil {
		return 0, err
	}
	return out.Myself.ClientBalance, nil
}

const podFields = `id desiredStatus costPerHr runtime { uptimeInSeconds ports { privatePort publicPort type } }`

// launch sobe um pod. `dockerArgs` é o comando de partida; `env` viaja como
// variáveis (script em base64, chaves), que é o que evita quoting frágil.
func (c *runpodClient) launch(ctx context.Context, gpu, cloud, image, dockerArgs, ports string, env map[string]string) (podInfo, error) {
	evs := make([]map[string]string, 0, len(env))
	for k, v := range env {
		evs = append(evs, map[string]string{"key": k, "value": v})
	}
	input := map[string]any{
		"cloudType":         cloud,
		"gpuCount":          1,
		"gpuTypeId":         gpu,
		"name":              "autodj-stemsep",
		"imageName":         image,
		"containerDiskInGb": 30,
		"volumeInGb":        0,
		"volumeMountPath":   "/workspace",
		"dockerArgs":        dockerArgs,
		"ports":             ports,
		"env":               evs,
	}
	var out struct {
		Pod podInfo `json:"podFindAndDeployOnDemand"`
	}
	q := `mutation Launch($input: PodFindAndDeployOnDemandInput!) {
  podFindAndDeployOnDemand(input: $input) { ` + podFields + ` }
}`
	if err := c.do(ctx, q, map[string]any{"input": input}, &out); err != nil {
		return podInfo{}, err
	}
	if out.Pod.ID == "" {
		return podInfo{}, ErrNoCapacity
	}
	return out.Pod, nil
}

// status consulta um pod. Devolve ok=false quando ele não existe mais — que é como
// o watchdog confirma a morte (a mutation de terminar não confirma nada).
func (c *runpodClient) status(ctx context.Context, id string) (podInfo, bool, error) {
	var out struct {
		Myself struct {
			Pods []podInfo `json:"pods"`
		} `json:"myself"`
	}
	if err := c.do(ctx, `query { myself { pods { `+podFields+` } } }`, nil, &out); err != nil {
		return podInfo{}, false, err
	}
	for _, p := range out.Myself.Pods {
		if p.ID == id {
			return p, true, nil
		}
	}
	return podInfo{}, false, nil
}

// terminate pede a morte do pod. NÃO confirma nada — ver o aviso no topo.
func (c *runpodClient) terminate(ctx context.Context, id string) error {
	q := fmt.Sprintf(`mutation { podTerminate(input:{podId:%q}) }`, id)
	return c.do(ctx, q, nil, nil)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

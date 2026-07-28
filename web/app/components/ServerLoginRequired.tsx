"use client";

/* Servidor exigindo login num build que não tem tela de login.

   O AutoDJ caseiro é single-user por decisão de produto: a estação é isolada por
   browser (fingerprint), não por conta, e ninguém faz login pra ouvir a própria
   rádio. A tela de login existia mesmo assim e só aparecia por ACIDENTE — o
   segredo JWT de dev é regerado a cada boot do servidor, então um restart
   invalidava o token guardado e o operador levava um formulário do nada ("que
   tela é essa que nem deveria existir?").

   O formulário saiu (0.2.26). O que fica é este aviso, para o único caso em que o
   servidor legitimamente recusa: alguém subiu com `AUTODJ_SINGLE_USER=0`
   (multiusuário) — aí o backend pede credencial que este build não sabe pedir.
   Uma tela em branco esconderia o motivo; esta diz o que houve e o que mudar. */
export function ServerLoginRequired() {
  return (
    <div className="app-boot srv-login">
      <div className="srv-login-card">
        <h1>AutoDJ</h1>
        <p>
          O servidor está em <strong>modo multiusuário</strong> e pediu credenciais — mas
          esta versão é de uso pessoal e não tem tela de login.
        </p>
        <p className="srv-login-hint">
          Suba o servidor com <code>AUTODJ_SINGLE_USER=1</code> (o padrão) para abrir o
          console direto.
        </p>
      </div>
    </div>
  );
}

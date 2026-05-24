/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Override the WebSocket URL used by the client.
   *  Defaults to ws://<hostname>:8080/ws when unset.
   *  Set to ws://localhost:9090/ws when running cmd/laggy. */
  readonly VITE_SERVER_WS_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

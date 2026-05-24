/**
 * Socket — minimal WebSocket wrapper with auto-reconnect and backoff.
 *
 * Usage:
 *   const sock = new Socket();
 *   sock.onMessage((data) => console.log(data));
 *   sock.connect();
 *   sock.send("hello");
 */

const BACKOFF_MS = [500, 1000, 2000, 4000, 5000];

type MessageCallback = (data: string) => void;

export class Socket {
  private readonly url: string;
  private ws: WebSocket | null = null;
  private queue: string[] = [];
  private subscribers: MessageCallback[] = [];
  private attempt = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(url?: string) {
    this.url =
      url ?? `ws://${location.hostname}:8080/ws`;
  }

  /** Open the WebSocket connection. Safe to call multiple times. */
  connect(): void {
    if (
      this.ws !== null &&
      (this.ws.readyState === WebSocket.OPEN ||
        this.ws.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }

    const ws = new WebSocket(this.url);
    this.ws = ws;

    ws.onopen = () => {
      console.log("WS connected");
      this.attempt = 0;
      // Flush queued messages.
      for (const msg of this.queue) {
        ws.send(msg);
      }
      this.queue = [];
    };

    ws.onmessage = (ev: MessageEvent<string>) => {
      console.log(`echo: ${ev.data}`);
      for (const cb of this.subscribers) {
        cb(ev.data);
      }
    };

    ws.onclose = () => {
      console.log("WS disconnected");
      this.ws = null;
      this.scheduleReconnect();
    };

    ws.onerror = (ev) => {
      console.error("WS error", ev);
    };
  }

  /** Send a text message. Queued if the socket is not yet open. */
  send(msg: string): void {
    if (this.ws !== null && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(msg);
    } else {
      this.queue.push(msg);
    }
  }

  /** Register a subscriber that is called for every received message. */
  onMessage(cb: MessageCallback): void {
    this.subscribers.push(cb);
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer !== null) {
      return;
    }
    const delay = BACKOFF_MS[Math.min(this.attempt, BACKOFF_MS.length - 1)];
    this.attempt += 1;
    console.log(`WS reconnecting in ${delay}ms (attempt ${this.attempt})`);
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }
}

/**
 * Socket — minimal WebSocket wrapper with auto-reconnect and backoff.
 *
 * Usage:
 *   const sock = new Socket();
 *   sock.onMessage((data) => console.log(data));
 *   sock.onBinaryMessage((buf) => handleBinary(buf));
 *   sock.onOpen(() => sender.start());
 *   sock.onClose(() => sender.stop());
 *   sock.connect();
 *   sock.send("hello");
 *   sock.sendBinary(new Uint8Array([1,2,3]));
 */

const BACKOFF_MS = [500, 1000, 2000, 4000, 5000];

type MessageCallback = (data: string) => void;
type BinaryCallback = (data: ArrayBuffer) => void;
type HookCallback = () => void;

export class Socket {
  private readonly url: string;
  private ws: WebSocket | null = null;
  private queue: string[] = [];
  private subscribers: MessageCallback[] = [];
  private binarySubscribers: BinaryCallback[] = [];
  private openHooks: HookCallback[] = [];
  private closeHooks: HookCallback[] = [];
  private attempt = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(url?: string) {
    this.url = url ?? `ws://${location.hostname}:8080/ws`;
  }

  /** Open the WebSocket connection. Safe to call multiple times. */
  connect(): void {
    if (
      this.ws !== null &&
      (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }

    const ws = new WebSocket(this.url);
    ws.binaryType = 'arraybuffer';
    this.ws = ws;

    ws.onopen = () => {
      console.log('WS connected');
      this.attempt = 0;
      // Flush queued text messages.
      for (const msg of this.queue) {
        ws.send(msg);
      }
      this.queue = [];
      for (const cb of this.openHooks) {
        cb();
      }
    };

    ws.onmessage = (ev: MessageEvent) => {
      if (ev.data instanceof ArrayBuffer) {
        for (const cb of this.binarySubscribers) {
          cb(ev.data);
        }
      } else {
        const text = ev.data as string;
        console.log(`echo: ${text}`);
        for (const cb of this.subscribers) {
          cb(text);
        }
      }
    };

    ws.onclose = () => {
      console.log('WS disconnected');
      this.ws = null;
      for (const cb of this.closeHooks) {
        cb();
      }
      this.scheduleReconnect();
    };

    ws.onerror = (ev) => {
      console.error('WS error', ev);
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

  /**
   * Send a binary frame. Returns true if the socket was open and the
   * frame was handed to the WS layer; false otherwise (frame dropped).
   */
  sendBinary(data: Uint8Array): boolean {
    if (this.ws !== null && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(data);
      return true;
    }
    return false;
  }

  /** Returns true when the underlying WebSocket is in the OPEN state. */
  isOpen(): boolean {
    return this.ws !== null && this.ws.readyState === WebSocket.OPEN;
  }

  /** Register a subscriber for text messages. */
  onMessage(cb: MessageCallback): void {
    this.subscribers.push(cb);
  }

  /** Register a subscriber for binary (ArrayBuffer) messages. */
  onBinaryMessage(cb: BinaryCallback): void {
    this.binarySubscribers.push(cb);
  }

  /** Register a hook called every time the connection opens. */
  onOpen(cb: HookCallback): void {
    this.openHooks.push(cb);
  }

  /** Register a hook called every time the connection closes. */
  onClose(cb: HookCallback): void {
    this.closeHooks.push(cb);
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

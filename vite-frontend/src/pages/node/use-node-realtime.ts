import { useCallback, useEffect, useRef, useState } from "react";
import axios from "axios";

import { getToken } from "@/utils/session";

interface NodeRealtimeMessage {
  id?: string | number;
  type?: string;
  data?: unknown;
  message?: string;
}

interface UseNodeRealtimeOptions {
  onMessage: (message: NodeRealtimeMessage) => void;
  enabled?: boolean;
}

const getRealtimeWsUrl = (): string => {
  const baseUrl =
    axios.defaults.baseURL ||
    (import.meta.env.VITE_API_BASE
      ? `${import.meta.env.VITE_API_BASE}/api/v1/`
      : "/api/v1/");

  return (
    baseUrl.replace(/^http/, "ws").replace(/\/api\/v1\/$/, "") +
    `/system-info?type=0&secret=${getToken() || ""}`
  );
};

export const useNodeRealtime = ({
  onMessage,
  enabled = true,
}: UseNodeRealtimeOptions) => {
  const [wsConnected, setWsConnected] = useState(false);
  const [wsConnecting, setWsConnecting] = useState(false);

  const websocketRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttemptsRef = useRef(0);
  const onMessageRef = useRef(onMessage);

  const maxReconnectAttempts = 5;

  useEffect(() => {
    onMessageRef.current = onMessage;
  }, [onMessage]);

  const clearReconnectTimer = useCallback(() => {
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
  }, []);

  const disconnect = useCallback(() => {
    clearReconnectTimer();
    reconnectAttemptsRef.current = 0;
    setWsConnected(false);
    setWsConnecting(false);

    if (!websocketRef.current) {
      return;
    }

    websocketRef.current.onopen = null;
    websocketRef.current.onmessage = null;
    websocketRef.current.onerror = null;
    websocketRef.current.onclose = null;

    if (
      websocketRef.current.readyState === WebSocket.OPEN ||
      websocketRef.current.readyState === WebSocket.CONNECTING
    ) {
      websocketRef.current.close();
    }

    websocketRef.current = null;
  }, [clearReconnectTimer]);

  const connect = useCallback(() => {
    if (!enabled) {
      return;
    }

    if (
      websocketRef.current &&
      (websocketRef.current.readyState === WebSocket.OPEN ||
        websocketRef.current.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }

    if (websocketRef.current) {
      disconnect();
    }

    try {
      setWsConnecting(true);
      websocketRef.current = new WebSocket(getRealtimeWsUrl());

      websocketRef.current.onopen = () => {
        reconnectAttemptsRef.current = 0;
        setWsConnected(true);
        setWsConnecting(false);
      };

      websocketRef.current.onmessage = (event) => {
        try {
          const parsed = JSON.parse(event.data);

          if (parsed && typeof parsed === "object") {
            onMessageRef.current(parsed as NodeRealtimeMessage);
          }
        } catch {}
      };

      websocketRef.current.onerror = () => {};

      websocketRef.current.onclose = () => {
        websocketRef.current = null;
        setWsConnected(false);
        setWsConnecting(false);

        if (!enabled || reconnectAttemptsRef.current >= maxReconnectAttempts) {
          return;
        }

        reconnectAttemptsRef.current += 1;
        reconnectTimerRef.current = setTimeout(() => {
          reconnectTimerRef.current = null;
          connect();
        }, 3000 * reconnectAttemptsRef.current);
      };
    } catch {
      setWsConnected(false);
      setWsConnecting(false);
    }
  }, [disconnect, enabled]);

  useEffect(() => {
    if (!enabled) {
      return;
    }

    connect();

    return () => {
      disconnect();
    };
  }, [connect, disconnect, enabled]);

  return {
    wsConnected,
    wsConnecting,
    reconnectRealtime: connect,
    disconnectRealtime: disconnect,
  };
};

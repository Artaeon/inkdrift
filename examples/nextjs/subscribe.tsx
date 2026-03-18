"use client";

import { useState, FormEvent } from "react";

interface InkDriftProps {
  apiUrl: string;
  listName?: string;
  className?: string;
}

export function NewsletterSubscribe({
  apiUrl,
  listName,
  className,
}: InkDriftProps) {
  const [email, setEmail] = useState("");
  const [status, setStatus] = useState<"idle" | "loading" | "success" | "error">("idle");
  const [message, setMessage] = useState("");

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setStatus("loading");

    try {
      const res = await fetch(`${apiUrl}/api/v1/subscribe`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          email,
          ...(listName ? { list: listName } : {}),
        }),
      });

      const data = await res.json();

      if (res.ok) {
        setStatus("success");
        setMessage(data.message || "Subscribed successfully!");
        setEmail("");
      } else {
        setStatus("error");
        setMessage(data.error || "Something went wrong");
      }
    } catch {
      setStatus("error");
      setMessage("Failed to connect to newsletter service");
    }
  }

  if (status === "success") {
    return (
      <div className={className}>
        <p>{message}</p>
      </div>
    );
  }

  return (
    <form onSubmit={handleSubmit} className={className}>
      <input
        type="email"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        placeholder="your@email.com"
        required
        disabled={status === "loading"}
      />
      <button type="submit" disabled={status === "loading"}>
        {status === "loading" ? "Subscribing..." : "Subscribe"}
      </button>
      {status === "error" && <p style={{ color: "red" }}>{message}</p>}
    </form>
  );
}

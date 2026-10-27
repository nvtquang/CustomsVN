import { createFileRoute } from "@tanstack/react-router";
import { ChatView } from "@/features/chat/chat-view";

export const Route = createFileRoute("/c/$id")({ component: Conversation });

function Conversation() {
  const { id } = Route.useParams();
  // Keyed so switching conversations starts a fresh view.
  return <ChatView key={id} id={Number(id)} />;
}

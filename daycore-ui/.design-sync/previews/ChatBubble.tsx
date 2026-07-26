import { ChatBubble } from "@daycore/ui";

export function Conversation() {
  return (
    <div className="flex w-80 flex-col gap-3">
      <ChatBubble role="assistant">早上好！我是 Leo，今天想从哪件事开始？</ChatBubble>
      <ChatBubble role="user">今天好累啊，事情有点多</ChatBubble>
      <ChatBubble role="assistant">嗯，先歇着。要不要我把下午的安排松一松</ChatBubble>
      <ChatBubble role="user">好，帮我把 3 点的会往后挪一小时</ChatBubble>
    </div>
  );
}

export function Assistant() {
  return (
    <div className="w-80">
      <ChatBubble role="assistant">
        我在你 18:00 之后留了一段空白，给自己一点喘息的时间。
      </ChatBubble>
    </div>
  );
}

export function User() {
  return (
    <div className="w-80">
      <ChatBubble role="user">把晚上的复习改成 21:00 开始</ChatBubble>
    </div>
  );
}

export function Typing() {
  return (
    <div className="w-80">
      <ChatBubble role="assistant" typing />
    </div>
  );
}

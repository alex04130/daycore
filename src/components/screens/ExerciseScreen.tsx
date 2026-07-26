"use client";

import { useState, useEffect, useRef } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { useRouter } from "next/navigation";
import { X, CheckCircle } from "lucide-react";

const EXERCISES = {
  breathing: {
    name: "4-7-8 呼吸",
    steps: [
      { label: "吸气", duration: 4, instruction: "用鼻子缓缓吸气…" },
      { label: "屏息", duration: 7, instruction: "轻轻屏住呼吸…" },
      { label: "呼气", duration: 8, instruction: "用嘴慢慢呼出…" },
    ],
    rounds: 3,
  },
  stretch: {
    name: "桌边伸展",
    steps: [
      { label: "左侧颈部", duration: 10, instruction: "头轻轻偏向左肩，保持…" },
      { label: "右侧颈部", duration: 10, instruction: "头轻轻偏向右肩，保持…" },
      { label: "双肩后展", duration: 10, instruction: "双手交叉放背后，胸部前展…" },
      { label: "手腕旋转", duration: 10, instruction: "缓缓转动手腕，放松…" },
      { label: "深呼吸", duration: 10, instruction: "闭眼深吸一口气，缓缓放松…" },
    ],
    rounds: 1,
  },
  grounding: {
    name: "五感着陆",
    steps: [
      { label: "5 样看到的", duration: 20, instruction: "环顾四周，慢慢说出5样你能看到的东西…" },
      { label: "4 种听到的", duration: 20, instruction: "闭眼聆听，找出4种你能听到的声音…" },
      { label: "3 个摸到的", duration: 15, instruction: "伸出手，感受3种不同质感…" },
      { label: "2 种闻到的", duration: 10, instruction: "轻轻吸气，感受2种气味…" },
      { label: "1 种尝到的", duration: 10, instruction: "感受口腔里的1种味道…" },
    ],
    rounds: 1,
  },
};

interface Props {
  type: "breathing" | "stretch" | "grounding";
  onComplete?: () => void;
}

export function ExerciseScreen({ type, onComplete }: Props) {
  const router = useRouter();
  const exercise = EXERCISES[type];
  const [currentRound, setCurrentRound] = useState(0);
  const [currentStep, setCurrentStep] = useState(0);
  const [timeLeft, setTimeLeft] = useState(exercise.steps[0].duration);
  const [done, setDone] = useState(false);
  const timerRef = useRef<NodeJS.Timeout | null>(null);

  const totalSteps = exercise.steps.length * exercise.rounds;
  const completedSteps = currentRound * exercise.steps.length + currentStep;
  const progress = totalSteps > 0 ? completedSteps / totalSteps : 0;

  const step = exercise.steps[currentStep];
  const maxRadius = 80;
  const circleProgress = type === "breathing" ? (step.duration - timeLeft) / step.duration : 1;

  useEffect(() => {
    timerRef.current = setInterval(() => {
      setTimeLeft((t) => {
        if (t <= 1) {
          // Move to next step
          const nextStep = currentStep + 1;
          if (nextStep >= exercise.steps.length) {
            const nextRound = currentRound + 1;
            if (nextRound >= exercise.rounds) {
              clearInterval(timerRef.current!);
              setDone(true);
              onComplete?.();
              return 0;
            }
            setCurrentRound(nextRound);
            setCurrentStep(0);
            return exercise.steps[0].duration;
          }
          setCurrentStep(nextStep);
          return exercise.steps[nextStep].duration;
        }
        return t - 1;
      });
    }, 1000);
    return () => clearInterval(timerRef.current!);
  }, [currentStep, currentRound, exercise, onComplete]);

  return (
    <div className="min-h-svh flex flex-col items-center justify-center px-6 relative"
      style={{ background: "linear-gradient(135deg, var(--color-bg-start), var(--color-bg-end))" }}>
      {/* Close */}
      <button
        onClick={() => router.back()}
        className="absolute top-12 right-5 w-10 h-10 glass-card flex items-center justify-center">
        <X size={18} style={{ color: "var(--color-text-muted)" }} />
      </button>

      <h2 className="text-lg font-semibold mb-2 text-center" style={{ color: "var(--color-text-primary)" }}>
        {exercise.name}
      </h2>
      {exercise.rounds > 1 && (
        <p className="text-sm mb-8" style={{ color: "var(--color-text-muted)" }}>
          第 {currentRound + 1} / {exercise.rounds} 轮
        </p>
      )}

      {!done ? (
        <div className="flex flex-col items-center gap-8">
          {/* Breathing circle */}
          {type === "breathing" && (
            <div className="relative w-52 h-52 flex items-center justify-center">
              <svg className="absolute inset-0 -rotate-90" width="208" height="208">
                <circle cx="104" cy="104" r={maxRadius} fill="none"
                  stroke="color-mix(in srgb, var(--color-primary) 12%, transparent)" strokeWidth="8" />
                <motion.circle
                  cx="104" cy="104" r={maxRadius} fill="none"
                  stroke="var(--color-primary)" strokeWidth="8"
                  strokeLinecap="round"
                  strokeDasharray={`${2 * Math.PI * maxRadius}`}
                  style={{ strokeDashoffset: `${2 * Math.PI * maxRadius * (1 - circleProgress)}` }}
                  transition={{ duration: 0.5 }}
                />
              </svg>
              <motion.div
                animate={{ scale: step.label === "吸气" ? 1.2 : step.label === "呼气" ? 0.85 : 1 }}
                transition={{ duration: step.duration, ease: "easeInOut" }}
                className="w-28 h-28 rounded-full"
                style={{ background: "color-mix(in srgb, var(--color-primary) 20%, transparent)" }}
              />
              <div className="absolute inset-0 flex flex-col items-center justify-center">
                <span className="text-lg font-semibold" style={{ color: "var(--color-primary)" }}>{step.label}</span>
                <span className="text-3xl font-bold tabular-nums" style={{ color: "var(--color-text-primary)" }}>{timeLeft}</span>
              </div>
            </div>
          )}

          {/* Non-breathing step display */}
          {type !== "breathing" && (
            <div className="w-48 h-48 rounded-full glass-card flex flex-col items-center justify-center gap-2">
              <span className="text-4xl font-bold tabular-nums" style={{ color: "var(--color-primary)" }}>{timeLeft}</span>
              <span className="text-sm font-medium text-center px-4" style={{ color: "var(--color-text-secondary)" }}>
                {step.label}
              </span>
            </div>
          )}

          <p className="text-sm text-center max-w-xs" style={{ color: "var(--color-text-secondary)" }}>
            {step.instruction}
          </p>

          {/* Progress */}
          <div className="w-48 h-1.5 rounded-full" style={{ background: "color-mix(in srgb, var(--color-primary) 15%, transparent)" }}>
            <motion.div className="h-full rounded-full" style={{ background: "var(--color-primary)" }}
              animate={{ width: `${progress * 100}%` }}
              transition={{ duration: 0.5 }} />
          </div>
        </div>
      ) : (
        <motion.div
          initial={{ scale: 0.8, opacity: 0 }}
          animate={{ scale: 1, opacity: 1 }}
          transition={{ type: "spring", stiffness: 300, damping: 20 }}
          className="flex flex-col items-center gap-4 text-center">
          <motion.div
            animate={{ scale: [1, 1.2, 1] }}
            transition={{ repeat: 2, duration: 0.4 }}>
            <CheckCircle size={64} style={{ color: "var(--color-states-success)" }} />
          </motion.div>
          <h3 className="text-xl font-bold" style={{ color: "var(--color-text-primary)" }}>完成了！</h3>
          <p className="text-sm" style={{ color: "var(--color-text-secondary)" }}>
            {exercise.name} 已完成，感觉怎么样？
          </p>
          <button
            onClick={() => router.back()}
            className="mt-4 px-6 py-3 rounded-[var(--radius-button)] font-medium text-white"
            style={{ background: "var(--color-primary)" }}>
            返回
          </button>
        </motion.div>
      )}
    </div>
  );
}

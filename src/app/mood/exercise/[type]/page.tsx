import { ExerciseScreen } from "@/components/screens/ExerciseScreen";

export default async function ExercisePage({ params }: { params: Promise<{ type: string }> }) {
  const { type } = await params;
  const validTypes = ["breathing", "stretch", "grounding"] as const;
  const exerciseType = validTypes.includes(type as typeof validTypes[number])
    ? (type as typeof validTypes[number])
    : "breathing";

  return <ExerciseScreen type={exerciseType} />;
}

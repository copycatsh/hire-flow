import { useState, type FormEvent } from "react";
import { useNavigate } from "@tanstack/react-router";
import { z } from "zod";
import { useAuth } from "@/features/auth/auth-context";
import { useCreateJob } from "./queries";

const schema = z
  .object({
    title: z.string().min(3, "Title must be at least 3 characters"),
    description: z
      .string()
      .min(10, "Description must be at least 10 characters"),
    budget_min: z.number().gt(0, "Minimum budget must be greater than 0"),
    budget_max: z.number().gt(0, "Maximum budget must be greater than 0"),
  })
  .refine((d) => d.budget_max >= d.budget_min, {
    message: "Maximum budget must be at least the minimum",
    path: ["budget_max"],
  });

type FieldErrors = Partial<Record<string, string>>;

export function CreateJobForm() {
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [budgetMin, setBudgetMin] = useState("");
  const [budgetMax, setBudgetMax] = useState("");
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});

  const { user } = useAuth();
  const createJob = useCreateJob();
  const navigate = useNavigate();

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setFieldErrors({});

    const result = schema.safeParse({
      title,
      description,
      budget_min: budgetMin === "" ? 0 : Number(budgetMin),
      budget_max: budgetMax === "" ? 0 : Number(budgetMax),
    });

    if (!result.success) {
      const errors: FieldErrors = {};
      for (const issue of result.error.issues) {
        const key = issue.path[0]?.toString();
        if (key && !errors[key]) {
          errors[key] = issue.message;
        }
      }
      setFieldErrors(errors);
      return;
    }

    if (!user) return;

    createJob.mutate(
      {
        title: result.data.title,
        description: result.data.description,
        budget_min: result.data.budget_min * 100,
        budget_max: result.data.budget_max * 100,
        client_id: user.user_id,
      },
      {
        onSuccess: (job) => navigate({ to: "/jobs/$id", params: { id: job.id } }),
      },
    );
  }

  const inputClass =
    "w-full rounded-sm border border-border-strong bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary-light";

  return (
    <div className="mx-auto max-w-lg">
      <h1 className="font-display text-xl font-semibold">Post a New Job</h1>

      {createJob.isError && (
        <div className="mt-4 rounded-sm bg-error-bg px-4 py-2 text-sm text-error">
          {createJob.error.message}
        </div>
      )}

      <form onSubmit={handleSubmit} className="mt-6 flex flex-col gap-4">
        <div>
          <label className="mb-1 block text-xs font-medium">Title</label>
          <input
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="e.g. Senior React Developer"
            className={inputClass}
          />
          {fieldErrors.title && (
            <p className="mt-1 text-xs text-error">{fieldErrors.title}</p>
          )}
        </div>

        <div>
          <label className="mb-1 block text-xs font-medium">Description</label>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Describe the role, requirements, and expectations..."
            rows={4}
            className={inputClass}
          />
          {fieldErrors.description && (
            <p className="mt-1 text-xs text-error">{fieldErrors.description}</p>
          )}
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="mb-1 block text-xs font-medium">
              Min Budget ($)
            </label>
            <input
              type="number"
              value={budgetMin}
              onChange={(e) => setBudgetMin(e.target.value)}
              placeholder="1000"
              className={inputClass}
            />
            {fieldErrors.budget_min && (
              <p className="mt-1 text-xs text-error">
                {fieldErrors.budget_min}
              </p>
            )}
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium">
              Max Budget ($)
            </label>
            <input
              type="number"
              value={budgetMax}
              onChange={(e) => setBudgetMax(e.target.value)}
              placeholder="5000"
              className={inputClass}
            />
            {fieldErrors.budget_max && (
              <p className="mt-1 text-xs text-error">
                {fieldErrors.budget_max}
              </p>
            )}
          </div>
        </div>

        <button
          type="submit"
          disabled={createJob.isPending}
          className="mt-1 w-full rounded-md bg-primary py-2.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:opacity-50"
        >
          {createJob.isPending ? "Posting..." : "Post Job"}
        </button>
      </form>
    </div>
  );
}

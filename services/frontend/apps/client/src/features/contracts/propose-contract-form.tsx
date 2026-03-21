import { useState, type FormEvent } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useAuth } from "@/features/auth/auth-context";
import { useCreateContract } from "./queries";

interface Props {
  freelancerId?: string;
  jobId?: string;
}

export function ProposeContractForm({ freelancerId, jobId }: Props) {
  const { user } = useAuth();
  const navigate = useNavigate();
  const createContract = useCreateContract();

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [amount, setAmount] = useState("");

  function handleSubmit(e: FormEvent) {
    e.preventDefault();

    const amountCents = Math.round(Number(amount) * 100);
    if (!user || !freelancerId || amountCents <= 0) return;

    createContract.mutate(
      {
        client_id: user.user_id,
        freelancer_id: freelancerId,
        title,
        description,
        amount: amountCents,
        currency: "USD",
        client_wallet_id: user.user_id,
        freelancer_wallet_id: freelancerId,
      },
      {
        onSuccess: (contract) => navigate({ to: "/contracts/$id", params: { id: contract.id } }),
      },
    );
  }

  return (
    <div className="mx-auto max-w-lg">
      <h1 className="font-display text-xl font-semibold tracking-tight">Propose a Contract</h1>

      {freelancerId && (
        <p className="mt-2 text-sm text-foreground-secondary">
          Freelancer: {freelancerId.slice(0, 8)}...
        </p>
      )}

      {jobId && (
        <p className="mt-1 text-sm text-foreground-secondary">
          Job: {jobId.slice(0, 8)}...
        </p>
      )}

      {createContract.isError && (
        <div className="mt-4 rounded-sm bg-error-bg px-4 py-2 text-sm text-error">
          {createContract.error.message}
        </div>
      )}

      <form onSubmit={handleSubmit} className="mt-6 flex flex-col gap-4">
        <div>
          <label className="mb-1 block text-xs font-medium">Title</label>
          <input
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Contract title"
            className="w-full rounded-sm border border-border-strong bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary-light"
            required
          />
        </div>

        <div>
          <label className="mb-1 block text-xs font-medium">Description</label>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Describe the scope of work"
            rows={4}
            className="w-full rounded-sm border border-border-strong bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary-light"
            required
          />
        </div>

        <div>
          <label className="mb-1 block text-xs font-medium">Amount (USD)</label>
          <input
            type="number"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            placeholder="0.00"
            min="0.01"
            step="0.01"
            className="w-full rounded-sm border border-border-strong bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary-light"
            required
          />
        </div>

        <div>
          <label className="mb-1 block text-xs font-medium">Currency</label>
          <span className="text-sm text-foreground-secondary">USD</span>
        </div>

        <button
          type="submit"
          disabled={createContract.isPending || !freelancerId}
          className="mt-1 w-full rounded-md bg-primary py-2.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:opacity-50"
        >
          {createContract.isPending ? "Submitting..." : "Submit Proposal"}
        </button>
      </form>
    </div>
  );
}

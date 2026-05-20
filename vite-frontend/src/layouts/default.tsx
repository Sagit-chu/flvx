import { Navbar } from "@/components/navbar";

export default function DefaultLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="relative flex flex-col min-h-screen bg-mesh-gradient">
      <Navbar />
      <main className="container mx-auto max-w-7xl flex-grow px-4 pt-4 sm:px-6 sm:pt-10">
        {children}
      </main>
    </div>
  );
}

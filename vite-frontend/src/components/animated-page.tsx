import { motion } from "framer-motion";
import React from "react";

/**
 * Wraps page content with a smooth fade-in + slide-up entrance animation.
 * Use this as the outermost wrapper inside each page component.
 */
export const AnimatedPage = ({
    children,
    className,
}: {
    children: React.ReactNode;
    className?: string;
}) => (
    <motion.div
        animate={{ opacity: 1, y: 0 }}
        className={className}
        exit={{ opacity: 0, y: -8 }}
        initial={{ opacity: 0, y: 16 }}
        transition={{ duration: 0.28, ease: [0.25, 0.46, 0.45, 0.94] }}
    >
        {children}
    </motion.div>
);

/**
 * Stagger container — apply to the parent of a list/grid of animated items.
 * Children should use `staggerItem` as their `variants` prop.
 */
export const staggerContainer = {
    hidden: { opacity: 0 },
    show: {
        opacity: 1,
        transition: {
            staggerChildren: 0.05,
            delayChildren: 0.05,
        },
    },
};

/**
 * Individual stagger item variant — fade-in + slide-up.
 */
export const staggerItem = {
    hidden: { opacity: 0, y: 12 },
    show: {
        opacity: 1,
        y: 0,
        transition: { duration: 0.25, ease: [0.25, 0.46, 0.45, 0.94] },
    },
};

/**
 * Convenience wrapper for a stagger list.
 * Renders a `motion.div` (or `motion.ul/motion.tbody`) with stagger behaviour.
 */
export const StaggerList = ({
    children,
    className,
    as = "div",
}: {
    children: React.ReactNode;
    className?: string;
    as?: "div" | "ul" | "tbody";
}) => {
    const Component = motion[as] as React.ElementType;

    return (
        <Component
            animate="show"
            className={className}
            initial="hidden"
            variants={staggerContainer}
        >
            {children}
        </Component>
    );
};

/**
 * Individual animated item for use inside StaggerList.
 */
export const StaggerItem = ({
    children,
    className,
    as = "div",
}: {
    children: React.ReactNode;
    className?: string;
    as?: "div" | "li" | "tr";
}) => {
    const Component = motion[as] as React.ElementType;

    return (
        <Component className={className} variants={staggerItem}>
            {children}
        </Component>
    );
};

/**
 * Simple fade-in animation for standalone elements (chips, badges, counters, etc.)
 * Uses opacity + translateY to avoid font blurriness caused by scale transforms.
 */
export const FadeIn = ({
    children,
    delay = 0,
    className,
}: {
    children: React.ReactNode;
    delay?: number;
    className?: string;
}) => (
    <motion.div
        animate={{ opacity: 1, y: 0 }}
        className={className}
        initial={{ opacity: 0, y: 8 }}
        transition={{ duration: 0.2, delay, ease: "easeOut" }}
    >
        {children}
    </motion.div>
);

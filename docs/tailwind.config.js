/** @type {import('tailwindcss').Config} */
export default {
  content: ["./src/**/*.{html,js,njk,md}"],
  darkMode: "media", // Enable dark mode based on prefers-color-scheme
  theme: {
    extend: {
      typography: {
        DEFAULT: {
          css: {
            maxWidth: "none",
            color: "#374151",
            a: {
              color: "#2563eb",
              "&:hover": {
                color: "#1d4ed8",
              },
            },
            "code::before": {
              content: '""',
            },
            "code::after": {
              content: '""',
            },
            code: {
              backgroundColor: "#f3f4f6",
              padding: "0.25rem 0.375rem",
              borderRadius: "0.25rem",
              fontWeight: "400",
            },
            "pre code": {
              backgroundColor: "transparent",
              padding: "0",
            },
          },
        },
        invert: {
          css: {
            color: "#d1d5db",
            "h1, h2, h3, h4, h5, h6": {
              color: "#f9fafb",
            },
            strong: {
              color: "#f9fafb",
            },
            a: {
              color: "#60a5fa",
              "&:hover": {
                color: "#93c5fd",
              },
            },
            code: {
              color: "#f9fafb",
              backgroundColor: "#374151",
            },
            "pre code": {
              backgroundColor: "transparent",
              padding: "0",
              color: "inherit",
            },
            blockquote: {
              color: "#d1d5db",
              borderLeftColor: "#4b5563",
            },
            "ol > li::marker": {
              color: "#9ca3af",
            },
            "ul > li::marker": {
              color: "#9ca3af",
            },
          },
        },
      },
    },
  },
  plugins: [require("@tailwindcss/typography")],
};

document.addEventListener("DOMContentLoaded", function () {
    // set up a SSE poll to /tail
    var eventSource = new EventSource("/debug/tail");
    eventSource.onmessage = function (event) {
        // Prepend the new data to the pre element (reverse chronological order)
        const tailElement = document.getElementById("tail");
        const timestamp = new Date().toISOString();
        tailElement.innerText = `[${timestamp}] ${event.data}\n` + tailElement.innerText;

        // Ensure the view is scrolled to the top (newest content)
        window.scrollTo(0, 0);
    };
    document.querySelector("form").addEventListener("submit", function (event) {
        event.preventDefault();

        // send the command
        fetch("/debug/send-command-api", {
            method: "POST",
            headers: {
                "Content-Type": "application/x-www-form-urlencoded",
            },
            body: new URLSearchParams({
                command: document.getElementById("command").value,
            }),
        })
            .catch((error) => {
                console.error("Error:", error);
            });
    });
});

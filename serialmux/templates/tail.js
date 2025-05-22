document.addEventListener("DOMContentLoaded", function () {
    // set up a SSE poll to /tail
    var eventSource = new EventSource("/debug/tail");
    eventSource.onmessage = function (event) {
        // append the response to the pre element
        document.getElementById("tail").innerText += event.data + "\n\n";
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
            .then((response) => {
                document.getElementById("api-response").innerText = response.text();
            })
            .catch((error) => {
                console.error("Error:", error);
            });
    });
});

# Quiz: prove I understand this change

Write questions a reviewer can only answer if they understood the hunk and its
consequences. No syntax trivia.

Prefer:

- "What happens when X is empty / nil / duplicated / retried?"
- "Which caller or downstream system breaks if this is wrong?"
- "Why does this ordering matter?"
- "What would it take to roll this back after it ships?"

Avoid questions answerable by re-reading a single line. Ask 3 questions, hardest last.
Read surrounding code in the workspace if needed to make the questions sharp.

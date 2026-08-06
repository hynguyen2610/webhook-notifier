# Next Change Steps

## Goal

- [ ] Refactor the documentation system so `README.md` becomes concise, high-signal, and structured around the highest-ROI project story

## README Target Structure

- [ ] Restructure `README.md` to follow this exact top-level order:
- [ ] `1. Overview`
- [ ] `2. Assignment Challenges`
- [ ] `3. Architecture`
- [ ] `4. Design Decisions`
- [ ] `5. Testing`
- [ ] `6. Future Work`
- [ ] Move supporting detail that is too long for the main README into appendix sections

## Assignment Challenges Section

- [ ] Present the three assignment challenges explicitly:
- [ ] `Reliability`
- [ ] `Fairness`
- [ ] `Horizontal scalability`
- [ ] For each challenge, add a short explanation of how the system addresses it
- [ ] For each challenge, link to the most relevant code
- [ ] For each challenge, link to the strongest available test or benchmark evidence when practical
- [ ] Keep this section concise and focused on highest-ROI understanding

## Architecture Section

- [ ] Summarize the runtime flow at a high level
- [ ] Keep architecture explanation short in the main README
- [ ] Link to deeper diagrams or supporting docs instead of duplicating large detail blocks

## Design Decisions Section

- [ ] Summarize the most important design decisions only
- [ ] Link to ADRs or trade-off docs for deeper explanation
- [ ] Avoid restating implementation details already covered better in code or appendix docs

## Testing Section

- [ ] Summarize the main automated test layers briefly
- [ ] Point to the strongest evidence for delivery, retry, fairness, and PostgreSQL-backed behavior
- [ ] Keep command examples minimal and useful

## Future Work Section

- [ ] Summarize the most important next improvements only
- [ ] Link to deeper future-looking docs instead of expanding the README too much

## Appendix Structure

- [ ] Add or reorganize appendix material so it contains:
- [ ] `A. Load Test Methodology`
- [ ] `B. Load Test Results`
- [ ] `C. Fairness Measurements`
- [ ] `D. Architecture Trade-offs`
- [ ] `E. Future Production Evolution`
- [ ] Ensure appendix items either live in clearly linked README appendix sections or in linked docs with matching labels

## Documentation Cleanup Rules

- [ ] Remove or move setup detail that makes the README feel too long for first-pass reading
- [ ] Keep the README optimized for fast project understanding rather than exhaustive operation notes
- [ ] Preserve important local run commands, but move lower-priority detail to appendix or supporting docs where appropriate
- [ ] Make sure links point to the most relevant existing docs before creating new duplication

## Verification

- [ ] Confirm the README is substantially more concise than before
- [ ] Confirm the README follows the requested section order
- [ ] Confirm each assignment challenge has short solution context plus code or evidence links
- [ ] Confirm appendix labels and linked supporting docs are easy to navigate

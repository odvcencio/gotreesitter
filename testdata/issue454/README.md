# Issue 454 field fixtures

This directory freezes the outside-user evidence from Issue 454.

Use `field_matrix_v1.json` as the identity record for graduation stage G1.
The record pins the candidate commit, grammar blobs, source rules, edit sites, and size bands.

The reporter published three generators verbatim:

- Python;
- JavaScript;
- TypeScript.

The fixture package implements only those exact generators.
Its tests use Secure Hash Algorithm 256-bit (SHA-256) digests to lock every source.

The repository also contains C, C#, and PHP regression fixtures.
Those fixtures use internal source templates and are not reporter-exact fixtures.

Keep G1 open until the reporter supplies every missing generator or source digest.
Do not use an internal approximation as outside-user acceptance evidence.

# Authorization

## Problem statement

Smith typically has a very broad set of permissions because it needs to be able to manage objects of various kinds
across several/many namespaces in a cluster. If permissions checks are not done properly, a malicious user may have
a way to escalate their privileges.
This document describes a way to address the problem of blocking ways to permissions escalation.

## Proposed solution

The idea is to capture identity of the user who creates/updates a `Bundle` and impersonate them when working with
objects of that `Bundle`. That way Smith becomes just a tool that can automate only the operations that the user
can perform already, it does not allow the user to do something that they are not allowed to do.

Implementation is straightforward.

1. Use a [MutatingAdmissionWebhook](https://kubernetes.io/docs/admin/admission-controllers/#mutatingadmissionwebhook-beta-in-19)
to capture information about the user's identity as a field in the `Bundle`.
2. [Impersonate the user](https://kubernetes.io/docs/admin/authentication/#user-impersonation) when making any requests
related to the `Bundle`. This includes reads from informers' caches/indexes.

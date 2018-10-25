# Soft deletes

## Problem statement

In a declarative system lack of description of a resource means that it should not exist. So if it does,
it should be deleted. This is a very powerful idea that needs some safety around it to prevent irreversible
consequences deletion typically has.

If resource is removed from a Bundle due to user error or a bug, it will be deleted once the Bundle reaches
ready state. To reduce impact of such mistake it is beneficial to have some sort of "soft deletion" mechanism.

## Proposed solution

Soft deletion should be opt-in based on resource definition in the Bundle. Soft deletion "flag" has to be part of
the object itself so that it is not lost once the object is removed from the Bundle. A natural place to put that bit
of information is an annotation on the object. `smith.a.c/SoftDelete=true/false`.

Smith already maintains a list of objects to be deleted in the Bundle's status. To record when a resource should
have been deleted, for "soft deletable" resources Smith will put a timestamp along the object's GVK and name in the
list. Timestamp is useful because it allows to build an external garbage collection mechanism.

Smith should never issue a delete for an object marked with a soft delete annotation.

Benefits of tracking timestamp in status vs as an annotation on the object:
- If the Bundle stops being a controller owner of an object, the timestamp is not preserved because the object
  is no longer a controlled object and hence is removed from the "objects to delete" list.
  I.e. cleanup is cheap and automatic, no annotations left on object. 
- If a controlled object is added back to the Bundle it is removed from the "objects to delete" list. No annotation
  on the object to clean up.
- Because there is no need to add/remove annotations on objects, less API calls are performed.

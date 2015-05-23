# refactor

A tool that inspects git repository and finds places for refactoring.

# Options

```
Usage of refactor:
  -after="1 week ago": inspect commits after that time
  -before="2015-05-22T19:14:16-07:00": inspect commits before that time
  -detail=false: show reason with only 1 count
  -reason=3: show top K reasons
  -target=10: show top K targets
```

# Output format

```
{score} {file1,file2} {related commit count}
{reason count} {reason1}
{reason count} {reason2}
```

# Sample

```
  4144.0 refs.c                                     31
       5 static int write_ref_sha1(struct ref_lock *lock,
       3 commit_ref_update(lock, sha1, logmsg))
       3 if (write_ref_to_lockfile(lock, sha1))

   126.0 remote.c                                    8

    56.0 remote.h                                    6

    44.0 sha1_name.c                                 6

    36.0 fetch-pack.c,upload-pack.c                  3

    35.0 builtin/for-each-ref.c                      4

    20.0 remote.c,remote.h                           3

    14.0 builtin/log.c                               2

    14.0 builtin/branch.c                            2

    12.0 log-tree.c                                  2
```

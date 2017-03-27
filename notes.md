## Implementing Callbacks

__Issues__
- A callback can only execute once concurrently
    - Different callbacks can be concurrent, but a given one can only execute once at a time
    - Use a mutex?

- Callbacks must be implemented in such a way that they don't cause `main` to exit
    - i.e. `minion.Run()` should have some kind of infinite loop
    - Simply loop forever or just wait for a signal?
    - Something like:

        ```
        sigc := make(chan os.Signal, 1)
        signal.Notify(sigc, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGHUP)
        log.Printf("Caught signal %s: shutting down.\n", <-c)
        os.Exit(0)
        ```

- Passing arguments
    - Our trigger loops require different arguments for their bodies
    - Either pass an `interface{}` and allow functions to use type assertions or use closures
        - Closures are probably cleaner and safer

- Options for debugging
    - 2 ways to register a trigger:
        - Regular trigger register, simply calls the function
        - Named trigger register, wraps the function in another that prints on entry, exit and times itself
    - Maybe like this?

        ```
        conn.Trigger(func() { fmt.Println("Hello") }, db.ContainerTable)
        ```

        Would print this:

        ```
        Hello
        ```

        but this:
        
        ```
        conn.Trigger(func() { fmt.Println("Hello") }, db.ContainerTable).Name("Test trigger")
        ```

        would print:

        ```
        Entering callback "Test trigger" (triggered by <cause>)
        Hello
        Exiting callback "Test trigger" (Elapsed time: 0m 0s 1ms)
        ```

        where cause is either "Timer" or "`<action>` on `<table>`" (i.e. "deletion from ContainerTable")

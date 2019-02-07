module Process exposing
  ( Id
  , spawn
  , sleep
  , kill
  )

{-|

# Processes
@docs Id, spawn, sleep, kill

## Future Plans

Right now, this library is pretty sparse. For example, there is no public API
for processes to communicate with each other. This is a really important
ability, but it is also something that is extraordinarily easy to get wrong!

I think the trend will be towards an Erlang style of concurrency, where every
process has an “event queue” that anyone can send messages to. I currently
think the API will be extended to be more like this:

    type Id exit msg

    spawn : Task exit a -> Task x (Id exit Never)

    kill : Id exit msg -> Task x ()

    send : Id exit msg -> msg -> Task x ()

A process `Id` will have two type variables to make sure all communication is
valid. The `exit` type describes the messages that are produced if the process
fails because of user code. So if processes are linked and trapping errors,
they will need to handle this. The `msg` type just describes what kind of
messages this process can be sent by strangers.

We shall see though! This is just a draft that does not cover nearly everything
it needs to, so the long-term vision for concurrency in Elm will be rolling out
slowly as I get more data and experience.

I ask that people bullish on compiling to node.js keep this in mind. I think we
can do better than the hopelessly bad concurrency model of node.js, and I hope
the Elm community will be supportive of being more ambitious, even if it takes
longer. That’s kind of what Elm is all about.
-}

import Basics exposing (Float, Never)
import Elm.Kernel.Scheduler
import Elm.Kernel.Process
import Platform
import Task exposing (Task)


{-| A light-weight process that runs concurrently. You can use `spawn` to
get a bunch of different tasks running in different processes. The Elm runtime
will interleave their progress. So if a task is taking too long, we will pause
it at an `andThen` and switch over to other stuff.

**Note:** We make a distinction between *concurrency* which means interleaving
different sequences and *parallelism* which means running different
sequences at the exact same time. For example, a
[time-sharing system](https://en.wikipedia.org/wiki/Time-sharing) is definitely
concurrent, but not necessarily parallel. So even though JS runs within a
single OS-level thread, Elm can still run things concurrently.
-}
type alias Id =
  Platform.ProcessId


{-| Run a task in its own light-weight process. In the following example,
`task1` and `task2` will be interleaved. If `task1` makes a long HTTP request
or is just taking a long time, we can hop over to `task2` and do some work
there.

    spawn task1
      |> Task.andThen (\_ -> spawn task2)

**Note:** This creates a relatively restricted kind of `Process` because it
cannot receive any messages. More flexibility for user-defined processes will
come in a later release!
-}
spawn : Task x a -> Task y Id
spawn =
  Elm.Kernel.Scheduler.spawn


{-| Block progress on the current process for the given number of milliseconds.
The JavaScript equivalent of this is [`setTimeout`][setTimeout] which lets you
delay work until later.

[setTimeout]: https://developer.mozilla.org/en-US/docs/Web/API/WindowTimers/setTimeout
-}
sleep : Float -> Task x ()
sleep =
  Elm.Kernel.Process.sleep


{-| Sometimes you `spawn` a process, but later decide it would be a waste to
have it keep running and doing stuff. The `kill` function will force a process
to bail on whatever task it is running. So if there is an HTTP request in
flight, it will also abort the request.
-}
kill : Id -> Task x ()
kill =
  Elm.Kernel.Scheduler.kill


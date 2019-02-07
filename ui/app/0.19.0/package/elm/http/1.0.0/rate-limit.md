## Custom Rate-Limiting Strategies

This package has `Http.RateLimit` which helps you rate-limit the HTTP requests you make. Instead of sending one request per keystroke, you filter it down because not all requests are important.

The `Http.RateLimit` module comes with a `debounce` strategy that covers the common case, but you may want to define a custom strategy with other characteristics. Maybe you want to send the first request. Maybe you want to send when the previous request is done instead of using timers. Etc.

If so, you can define a custom strategy with `Http.RateLimit.customStrategy`. For example, you would define `throttle` like this:

```elm
import Http.RateLimit as Limit

throttle : Time -> Limit.Strategy
throttle ms =
  Limit.customStrategy <| \timeNow event state ->
    case event of
      Limit.New _ ->
        -- wait after a new request
        [ Limit.WakeUpIn ms ]

      Limit.Done _ ->
        -- we do not care when requests finish
        []

      Limit.WakeUp ->
        case state.next of
          Nothing ->
            -- do nothing if there is no pending request
            []

          Just req ->
            -- send if enough time has passed since the previous request
            case state.prev of
              Nothing ->
                [ Limit.Send req.id ]

              Just prev ->
                if timeNow - prev.time >= ms then [ Limit.Send req.id ] else []
```

It would be nice to have some useful strategies defined in a separate package so folks can experiment and find names and implementations that work well for specific scenarios.
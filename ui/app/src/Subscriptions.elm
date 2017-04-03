module Subscriptions exposing (subscriptions)

import Keyboard
import Types exposing (Model, Msg(KeyDownMsg, KeyUpMsg))


subscriptions : Model -> Sub Msg
subscriptions model =
    Sub.batch
        [ Keyboard.downs KeyDownMsg
        , Keyboard.ups KeyUpMsg
        ]

port module Views.Settings.Updates exposing (..)

import Maybe
import String
import Task
import Types exposing (Msg(..))
import Views.Settings.Types exposing (..)
import Views.SilenceForm.Types exposing (SilenceFormMsg(..))


update : SettingsMsg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        UpdateStartWeekAtMonday startOfWeekString ->
            let
                monday =
                    startOfWeekString == "1"

                startOfWeek =
                    Maybe.withDefault 7
                        (String.toInt
                            startOfWeekString
                        )
            in
            ( { model | startOfWeek = startOfWeek }
            , Cmd.batch
                [ Task.perform identity
                    (Task.succeed
                        (MsgForSilenceForm
                            (UpdateFirstDayOfWeek
                                startOfWeek
                            )
                        )
                    )
                , persistStartWeekAtMonday monday
                ]
            )


port persistStartWeekAtMonday : Bool -> Cmd msg

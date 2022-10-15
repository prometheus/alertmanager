port module Views.Settings.Updates exposing (..)

import Task
import Types exposing (Msg(..))
import Utils.DateTimePicker.Utils exposing (FirstDayOfWeek(..))
import Views.Settings.Types exposing (..)
import Views.SilenceForm.Types


update : SettingsMsg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        UpdateStartWeekAtMonday startOfWeekString ->
            let
                startOfWeek =
                    case startOfWeekString of
                        "Monday" ->
                            Monday

                        "Sunday" ->
                            Sunday

                        _ ->
                            Monday

                startOfWeekString2 =
                    case startOfWeek of
                        Monday ->
                            "Monday"

                        Sunday ->
                            "Sunday"
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
                , persistStartWeekAtMonday startOfWeekString2
                ]
            )


port persistStartWeekAtMonday : String -> Cmd msg

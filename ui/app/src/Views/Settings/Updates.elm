port module Views.Settings.Updates exposing (..)

import Task
import Types exposing (Msg(..))
import Utils.DateTimePicker.Utils exposing (FirstDayOfWeek(..))
import Views.Settings.Types exposing (..)
import Views.SilenceForm.Types


update : SettingsMsg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        UpdateFirstDayOfWeek firstDayOfWeekString ->
            let
                firstDayOfWeek =
                    case firstDayOfWeekString of
                        "Monday" ->
                            Monday

                        "Sunday" ->
                            Sunday

                        _ ->
                            Monday

                firstDayOfWeekString2 =
                    case firstDayOfWeek of
                        Monday ->
                            "Monday"

                        Sunday ->
                            "Sunday"
            in
            ( { model | firstDayOfWeek = firstDayOfWeek }
            , Cmd.batch
                [ Task.perform identity
                    (Task.succeed
                        (MsgForSilenceForm
                            (Views.SilenceForm.Types.UpdateFirstDayOfWeek
                                firstDayOfWeek
                            )
                        )
                    )
                , persistFirstDayOfWeek firstDayOfWeekString2
                ]
            )

        UpdateBootstrapTheme themeString ->
            let
                validTheme =
                    case themeString of
                        "auto" ->
                            "auto"

                        "light" ->
                            "light"

                        "dark" ->
                            "dark"

                        _ ->
                            model.bootstrapTheme

                -- Default to the current theme if invalid
            in
            ( { model | bootstrapTheme = validTheme }
            , Task.perform identity (Task.succeed (SetAndPersistBootstrapTheme validTheme))
            )


port persistFirstDayOfWeek : String -> Cmd msg

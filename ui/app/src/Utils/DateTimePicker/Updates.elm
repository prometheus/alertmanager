module Utils.DateTimePicker.Updates exposing (update)

import Time exposing (Posix)
import Utils.DateTimePicker.Types exposing (DateTimePicker(..), InputHourOrMinute(..), Model, Msg(..), Settings, StartOrEnd(..), Status(..))
import Utils.DateTimePicker.Utils
    exposing
        ( addDateAndTime
        , addMaybePosix
        , determineDateTimeRange
        , doDaysMatch
        , pickUpTimeFromDateTimePosix
        , validRuntimeOrNothing
        )


update : Settings msg -> Msg -> DateTimePicker -> ( DateTimePicker, Maybe ( Posix, Posix ) )
update settings msg (DateTimePicker model) =
    case model.status of
        Open baseTime ->
            case msg of
                NextMonth ->
                    ( DateTimePicker { model | pickerOffset = model.pickerOffset + 1 }, Nothing )

                PrevMonth ->
                    ( DateTimePicker { model | pickerOffset = model.pickerOffset - 1 }, Nothing )

                NextYear ->
                    ( DateTimePicker { model | pickerOffset = model.pickerOffset + 12 }, Nothing )

                PrevYear ->
                    ( DateTimePicker { model | pickerOffset = model.pickerOffset - 12 }, Nothing )

                SetHoveredDay time ->
                    ( DateTimePicker { model | hovered = Just time }, Nothing )

                SetRange ->
                    case ( model.pickedStart, model.pickedEnd ) of
                        ( Just s, Just e ) ->
                            Maybe.map
                                (\hovered ->
                                    if doDaysMatch settings.zone e hovered then
                                        ( DateTimePicker { model | pickedStart = addDateAndTime settings.zone (Just s) model.timeStart, pickedEnd = Nothing }, Nothing )

                                    else if doDaysMatch settings.zone s hovered then
                                        ( DateTimePicker { model | pickedStart = Nothing, pickedEnd = Just e }, Nothing )

                                    else
                                        ( DateTimePicker { model | pickedStart = addDateAndTime settings.zone (Just hovered) model.timeStart, pickedEnd = Nothing }, Nothing )
                                )
                                model.hovered
                                |> Maybe.withDefault ( DateTimePicker model, Nothing )

                        _ ->
                            let
                                ( dateStart, dateEnd ) =
                                    determineDateTimeRange settings.zone model.pickedStart model.pickedEnd model.hovered

                                start =
                                    addDateAndTime settings.zone dateStart model.timeStart

                                end =
                                    addDateAndTime settings.zone dateEnd model.timeEnd
                            in
                            ( DateTimePicker { model | pickedStart = start, pickedEnd = end }, validRuntimeOrNothing start end )

                SetInputTime startOrEnd hourOrMinute inputInt ->
                    let
                        toMillis =
                            case hourOrMinute of
                                InputHour ->
                                    1000 * 60 * 60

                                InputMinute ->
                                    1000 * 60

                        toHourOrMinute =
                            case hourOrMinute of
                                InputHour ->
                                    Time.toHour

                                InputMinute ->
                                    Time.toMinute

                        picked =
                            \s ->
                                case s of
                                    Just time ->
                                        toHourOrMinute settings.zone time
                                            |> (\h ->
                                                    (inputInt - h)
                                                        |> (\diff -> diff * toMillis)
                                                        |> (\m -> m + Time.posixToMillis time)
                                               )
                                            |> Time.millisToPosix
                                            |> Just

                                    Nothing ->
                                        s
                    in
                    case startOrEnd of
                        Start ->
                            let
                                pickedStart =
                                    picked model.pickedStart

                                timeStart =
                                    pickUpTimeFromDateTimePosix settings.zone pickedStart
                            in
                            ( DateTimePicker { model | pickedStart = pickedStart, timeStart = timeStart }, validRuntimeOrNothing pickedStart model.pickedEnd )

                        End ->
                            let
                                pickedEnd =
                                    picked model.pickedEnd

                                timeEnd =
                                    pickUpTimeFromDateTimePosix settings.zone pickedEnd
                            in
                            ( DateTimePicker { model | pickedEnd = pickedEnd, timeEnd = timeEnd }, validRuntimeOrNothing model.pickedStart pickedEnd )

                IncrementTime startOrEnd hourOrMinute num ->
                    let
                        diffPosix =
                            case hourOrMinute of
                                InputHour ->
                                    1000 * 60 * 60 * num |> Time.millisToPosix

                                InputMinute ->
                                    1000 * 60 * num |> Time.millisToPosix
                    in
                    case startOrEnd of
                        Start ->
                            ( DateTimePicker
                                { model
                                    | pickedStart = addMaybePosix model.pickedStart diffPosix
                                    , timeStart = addMaybePosix model.timeStart diffPosix
                                }
                            , validRuntimeOrNothing (addMaybePosix model.pickedStart diffPosix) model.pickedEnd
                            )

                        End ->
                            ( DateTimePicker
                                { model
                                    | pickedEnd = addMaybePosix model.pickedEnd diffPosix
                                    , timeEnd = addMaybePosix model.timeEnd diffPosix
                                }
                            , validRuntimeOrNothing model.pickedStart (addMaybePosix model.pickedEnd diffPosix)
                            )

                Close ->
                    ( DateTimePicker { model | status = Closed }, Nothing )

                Cancel ->
                    ( DateTimePicker
                        { model
                            | status = Closed
                            , pickedStart = model.tmpStart
                            , pickedEnd = model.tmpEnd
                        }
                    , validRuntimeOrNothing model.tmpStart model.tmpEnd
                    )

        Closed ->
            ( DateTimePicker model, Nothing )

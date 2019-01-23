module Views.GroupBar.Views exposing (view)

import Html exposing (Attribute, Html, a, button, div, i, input, li, small, span, text, ul)
import Html.Attributes exposing (class, disabled, href, id, style, value)
import Html.Events exposing (keyCode, on, onBlur, onClick, onFocus, onInput, onMouseEnter, onMouseLeave)
import Set
import Utils.Keyboard exposing (keys, onKeyDown, onKeyUp)
import Utils.List
import Views.GroupBar.Types exposing (Model, Msg(..))


view : Model -> Html Msg
view ({ list, fieldText, fields } as model) =
    let
        isDisabled =
            not (Set.member fieldText list) || List.member fieldText fields

        className =
            if String.isEmpty fieldText then
                ""

            else if isDisabled then
                "has-danger"

            else
                "has-success"
    in
    div
        [ class "row no-gutters align-items-start" ]
        (List.map viewField fields
            ++ [ div
                    [ class ("col " ++ className)
                    , style "min-width" "200px"
                    ]
                    [ textInputField isDisabled model
                    , exampleField fields
                    , autoCompleteResults model
                    ]
               ]
        )


exampleField : List String -> Html Msg
exampleField fields =
    if List.member "alertname" fields then
        small [ class "form-text text-muted" ]
            [ text "Label key for grouping alerts"
            ]

    else
        small [ class "form-text text-muted" ]
            [ text "Label key for grouping alerts, e.g."
            , button
                [ class "btn btn-link btn-sm align-baseline"
                , onClick (UpdateFieldText "alertname")
                ]
                [ text "alertname" ]
            ]


textInputField : Bool -> Model -> Html Msg
textInputField isDisabled { fieldText, matches, maybeSelectedMatch, fields, backspacePressed } =
    let
        onClickMsg =
            if isDisabled then
                Noop

            else
                AddField True fieldText

        nextMatch =
            maybeSelectedMatch
                |> Maybe.map ((\b a -> Utils.List.nextElem a b) <| matches)
                |> Maybe.withDefault (List.head matches)

        prevMatch =
            maybeSelectedMatch
                |> Maybe.map ((\b a -> Utils.List.nextElem a b) <| List.reverse matches)
                |> Maybe.withDefault (Utils.List.lastElem matches)

        keyDown key =
            if key == keys.down then
                Select nextMatch

            else if key == keys.up then
                Select prevMatch

            else if key == keys.enter then
                if not isDisabled then
                    AddField True fieldText

                else
                    maybeSelectedMatch
                        |> Maybe.map (AddField True)
                        |> Maybe.withDefault Noop

            else if key == keys.backspace then
                if fieldText == "" then
                    case ( Utils.List.lastElem fields, backspacePressed ) of
                        ( Just lastField, False ) ->
                            DeleteField True lastField

                        _ ->
                            Noop

                else
                    PressingBackspace True

            else
                Noop

        keyUp key =
            if key == keys.backspace then
                PressingBackspace False

            else
                Noop
    in
    div [ class "input-group" ]
        [ input
            [ id "group-by-field"
            , class "form-control"
            , value fieldText
            , onKeyDown keyDown
            , onKeyUp keyUp
            , onInput UpdateFieldText
            , onFocus (Focus True)
            , onBlur (Focus False)
            ]
            []
        , span
            [ class "input-group-btn" ]
            [ button [ class "btn btn-primary", disabled isDisabled, onClick onClickMsg ] [ text "+" ] ]
        ]


autoCompleteResults : Model -> Html Msg
autoCompleteResults { maybeSelectedMatch, focused, resultsHovered, matches } =
    let
        autoCompleteClass =
            if (focused || resultsHovered) && not (List.isEmpty matches) then
                "show"

            else
                ""
    in
    div
        [ class ("autocomplete-menu " ++ autoCompleteClass)
        , onMouseEnter (ResultsHovered True)
        , onMouseLeave (ResultsHovered False)
        ]
        [ matches
            |> List.map (matchedField maybeSelectedMatch)
            |> div [ class "dropdown-menu" ]
        ]


matchedField : Maybe String -> String -> Html Msg
matchedField maybeSelectedMatch field =
    let
        className =
            if maybeSelectedMatch == Just field then
                "active"

            else
                ""
    in
    button
        [ class ("dropdown-item " ++ className)
        , onClick (AddField True field)
        ]
        [ text field ]


viewField : String -> Html Msg
viewField field =
    div [ class "col col-auto" ]
        [ div [ class "btn-group mr-2 mb-2" ]
            [ button
                [ class "btn btn-outline-info"
                , onClick (DeleteField True field)
                ]
                [ text field
                ]
            , button
                [ class "btn btn-outline-danger"
                , onClick (DeleteField False field)
                ]
                [ text "×" ]
            ]
        ]

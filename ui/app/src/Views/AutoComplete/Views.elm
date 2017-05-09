module Views.AutoComplete.Views exposing (view)

import Views.AutoComplete.Types exposing (Msg(..), Model)
import Html exposing (Html, Attribute, div, span, input, text, button, i, small, ul, li, a)
import Html.Attributes exposing (value, class, style, disabled, id, href)
import Html.Events exposing (onClick, onInput, on, keyCode, onFocus, onBlur)
import Set
import Utils.List
import Utils.Keyboard exposing (keys, onKeyUp, onKeyDown)


view : Model -> Html Msg
view { list, fieldText, fields, matches, maybeSelectedMatch, backspacePressed, focused } =
    let
        isDisabled =
            (list
                |> Set.toList
                |> List.member fieldText
                |> not
            )
                || (List.member fieldText fields)

        maybeLastField =
            Utils.List.lastElem fields

        nextMatch =
            maybeSelectedMatch
                |> Maybe.map (flip Utils.List.nextElem <| matches)
                |> Maybe.withDefault (List.head matches)

        prevMatch =
            maybeSelectedMatch
                |> Maybe.map (flip Utils.List.nextElem <| List.reverse matches)
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
                    case ( backspacePressed, maybeLastField ) of
                        ( False, Just lastField ) ->
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

        -- TODO: Interacting with autocomplete box
        -- * Scrolling with keyboard
        -- * Scrolling with keyboard + hitting enter
        -- TODO: Text needs to match one autocomplete field exactly
        onClickMsg =
            if isDisabled then
                Noop
            else
                AddField True fieldText

        className =
            if String.isEmpty fieldText then
                ""
            else if isDisabled then
                "has-danger"
            else
                "has-success"

        autoCompleteClass =
            if focused && not (List.isEmpty matches) then
                "show"
            else
                ""
    in
        div
            [ class "row no-gutters align-items-start pb-4" ]
            (viewFields fields
                ++ [ div
                        [ class ("col form-group " ++ className)
                        , style
                            [ ( "padding", "5px" )
                            , ( "min-width", "200px" )
                            , ( "max-width", "500px" )
                            ]
                        ]
                        [ div [ class "input-group" ]
                            [ input
                                [ id "auto-complete-field"
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
                                [ button [ class "btn btn-primary", disabled isDisabled, onClick onClickMsg ] [ text "Add" ] ]
                            ]
                        , small [ class "form-text text-muted" ]
                            [ text "Label keys for grouping alerts"
                            ]
                        , div [ class ("autocomplete-menu " ++ autoCompleteClass) ]
                            [ matches
                                |> List.map (matchedField maybeSelectedMatch)
                                |> div [ class "dropdown-menu" ]
                            ]
                        ]
                   ]
            )


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


viewFields : List String -> List (Html Msg)
viewFields fields =
    fields
        |> List.map viewField


viewField : String -> Html Msg
viewField field =
    div [ class "col col-auto", style [ ( "padding", "5px" ) ] ]
        [ div [ class "btn-group" ]
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

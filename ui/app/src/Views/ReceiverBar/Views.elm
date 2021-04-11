module Views.ReceiverBar.Views exposing (view)

import Html exposing (Html, div, input, li, text)
import Html.Attributes exposing (class, id, style, tabindex, value)
import Html.Events exposing (onBlur, onClick, onInput, onMouseEnter, onMouseLeave)
import Utils.Keyboard exposing (keys, onKeyDown)
import Utils.List
import Views.ReceiverBar.Types exposing (Model, Msg(..), Receiver)


view : Maybe String -> Model -> Html Msg
view maybeRegex model =
    if model.showReceivers || model.resultsHovered then
        viewDropdown model

    else
        viewResult maybeRegex model.receivers


viewResult : Maybe String -> List Receiver -> Html Msg
viewResult maybeRegex receivers =
    let
        unescapedReceiver =
            receivers
                |> List.filter (.regex >> Just >> (==) maybeRegex)
                |> List.map (.name >> Just)
                |> List.head
                |> Maybe.withDefault maybeRegex
    in
    li
        [ class "nav-item ml-auto"
        , tabindex 1
        , style "position" "relative"
        , style "outline" "none"
        ]
        [ div
            [ onClick EditReceivers
            , class "mt-1 mr-4"
            , style "cursor" "pointer"
            ]
            [ text ("Receiver: " ++ Maybe.withDefault "All" unescapedReceiver) ]
        ]


viewDropdown : Model -> Html Msg
viewDropdown { matches, fieldText, selectedReceiver } =
    let
        nextMatch =
            selectedReceiver
                |> Maybe.map ((\b a -> Utils.List.nextElem a b) <| matches)
                |> Maybe.withDefault (List.head matches)

        prevMatch =
            selectedReceiver
                |> Maybe.map ((\b a -> Utils.List.nextElem a b) <| List.reverse matches)
                |> Maybe.withDefault (Utils.List.lastElem matches)

        keyDown key =
            if key == keys.down then
                Select nextMatch

            else if key == keys.up then
                Select prevMatch

            else if key == keys.enter then
                selectedReceiver
                    |> Maybe.map .regex
                    |> Maybe.withDefault fieldText
                    |> FilterByReceiver

            else
                Noop
    in
    li
        [ class "nav-item ml-auto mr-4 autocomplete-menu show"
        , onMouseEnter (ResultsHovered True)
        , onMouseLeave (ResultsHovered False)
        , style "position" "relative"
        , style "outline" "none"
        ]
        [ input
            [ id "receiver-field"
            , value fieldText
            , onBlur BlurReceiverField
            , onInput UpdateReceiver
            , onKeyDown keyDown
            , class "mr-4"
            , style "display" "block"
            , style "width" "100%"
            ]
            []
        , matches
            |> List.map (receiverField selectedReceiver)
            |> div [ class "dropdown-menu dropdown-menu-right" ]
        ]


receiverField : Maybe Receiver -> Receiver -> Html Msg
receiverField selected receiver =
    let
        attrs =
            if selected == Just receiver then
                [ class "dropdown-item active" ]

            else
                [ class "dropdown-item"
                , style "cursor" "pointer"
                , onClick (FilterByReceiver receiver.regex)
                ]
    in
    div attrs [ text receiver.name ]
